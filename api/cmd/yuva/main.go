package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	netmail "net/mail"
	"os"
	"os/signal"
	"runtime/debug"
	"strings"
	"syscall"
	"time"
	_ "time/tzdata"

	"github.com/go-webauthn/webauthn/webauthn"

	"github.com/productdevbook/yuva/api/internal/api"
	"github.com/productdevbook/yuva/api/internal/config"
	"github.com/productdevbook/yuva/api/internal/jobs"
	"github.com/productdevbook/yuva/api/internal/mail"
	"github.com/productdevbook/yuva/api/internal/metrics"
	"github.com/productdevbook/yuva/api/internal/push"
	"github.com/productdevbook/yuva/api/internal/realtime"
	"github.com/productdevbook/yuva/api/internal/secret"
	"github.com/productdevbook/yuva/api/internal/storage"
	"github.com/productdevbook/yuva/api/internal/store"
)

var version = ""

const usageText = `usage:
  yuva serve [--migrate=false]   (applies database migrations first unless disabled)
  yuva migrate up|down|status
  yuva bootstrap --email <address> --workspace <name> [--name <name>] [--locale en|tr] [--allow-existing]
  yuva ingest-email --to <address> [--from <address>] < message.eml
  yuva api-key create --workspace <id|name> --name <name>   (prints only the secret, once)
  yuva api-key list --workspace <id|name>
  yuva api-key revoke <id>
  yuva inbox create --workspace <id|name> --name <name> [--slug <slug>] [--locale <tag>] [--timezone <zone>] [--mode live|async] [--expected-reply-minutes <n>]
  yuva inbox list --workspace <id|name>
  yuva channel create-email --workspace <id|name> --inbox <id|slug> --name <name> --address <address> [--display-name <name>] [--from-address <address>]
      [--smtp-host <host> [--smtp-port <n>] [--tls starttls|tls|none] [--smtp-username <name>] [--smtp-password-file <path|->]]
  yuva channel list --workspace <id|name> --inbox <id|slug>
  yuva workspace delete --workspace <id|name> --yes
  yuva person delete --email <address> --yes
  yuva vapid-keys                (prints a new Web Push key pair)`

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	slog.SetDefault(log)
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, usageText)
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if os.Args[1] == "vapid-keys" {
		if err := vapidKeys(); err != nil {
			log.Error("fatal", slog.Any("error", err))
			os.Exit(1)
		}
		return
	}
	cfg, err := config.Load(buildVersion())
	if err == nil {
		switch os.Args[1] {
		case "serve":
			err = serve(ctx, cfg, log, os.Args[2:])
		case "migrate":
			err = migrate(ctx, cfg, log, os.Args[2:])
		case "bootstrap":
			err = bootstrap(ctx, cfg, log, os.Args[2:])
		case "api-key":
			err = apiKeyCLI(ctx, cfg, os.Args[2:])
		case "inbox", "channel", "workspace", "person":
			err = operatorCLI(ctx, cfg, os.Args[1], os.Args[2:])
		case "ingest-email":
			os.Exit(ingestEmail(ctx, cfg, log, os.Args[2:]))
		default:
			fmt.Fprintln(os.Stderr, usageText)
			os.Exit(2)
		}
	}
	if err != nil {
		log.Error("fatal", slog.Any("error", err))
		os.Exit(1)
	}
}

func buildVersion() string {
	if version != "" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, s := range info.Settings {
			if s.Key == "vcs.revision" && len(s.Value) >= 12 {
				return s.Value[:12]
			}
		}
	}
	return "dev"
}

func openStore(ctx context.Context, cfg config.Config) (*store.Store, error) {
	if err := cfg.RequireDatabase(); err != nil {
		return nil, err
	}
	st, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, err
	}
	waitCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	if err := st.WaitReady(waitCtx); err != nil {
		st.Close()
		return nil, err
	}
	return st, nil
}

func migrateUp(ctx context.Context, st *store.Store, log *slog.Logger) error {
	if err := st.MigrateUp(ctx, log); err != nil {
		return err
	}
	return jobs.Migrate(ctx, st.Pool, log)
}

func serve(ctx context.Context, cfg config.Config, log *slog.Logger, args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	runMigrations := fs.Bool("migrate", true, "apply database migrations before serving")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := cfg.RequireMasterKey(); err != nil {
		return err
	}
	masterKey, err := secret.ParseKey(cfg.MasterKey)
	if err != nil {
		return fmt.Errorf("YUVA_MASTER_KEY: %w", err)
	}
	objects, err := openStorage(cfg.Storage)
	if err != nil {
		return err
	}
	pushKeys := push.Keys{PublicKey: cfg.VAPID.PublicKey, PrivateKey: cfg.VAPID.PrivateKey, Subject: cfg.VAPID.Subject}
	if cfg.VAPID.Enabled() {
		if err := pushKeys.Check(); err != nil {
			return fmt.Errorf("YUVA_VAPID_PRIVATE_KEY, YUVA_VAPID_PUBLIC_KEY: %w", err)
		}
	} else {
		log.Warn("YUVA_VAPID_PUBLIC_KEY is not set; Web Push notifications are off (yuva vapid-keys makes a pair)")
	}
	st, err := openStore(ctx, cfg)
	if err != nil {
		return err
	}
	defer st.Close()
	if *runMigrations {
		if err := migrateUp(ctx, st, log); err != nil {
			return err
		}
	}
	hub := realtime.NewHub(256)
	go realtime.Listen(ctx, cfg.DatabaseURL, st.Queries, hub, log)
	if cfg.MetricsAddr != "" {
		go metrics.Serve(ctx, cfg.MetricsAddr, log)
	}
	wa, err := webauthn.New(&webauthn.Config{
		RPID:          cfg.WebAuthnRPID,
		RPDisplayName: cfg.WebAuthnRPName,
		RPOrigins:     cfg.WebAuthnOrigins,
	})
	if err != nil {
		return fmt.Errorf("webauthn: %w", err)
	}
	var mailer mail.Mailer = mail.Log{Log: log}
	if cfg.SMTP.Enabled() {
		mailer = mail.SMTP{Config: cfg.SMTP}
	} else {
		log.Warn("YUVA_SMTP_HOST is not set; e-mails are written to the log instead of being sent")
	}
	srv := api.New(api.Deps{
		Log:      log,
		Store:    st,
		Version:  cfg.Version,
		Mailer:   mail.Async{Mailer: mailer, Log: log, Timeout: 30 * time.Second},
		WebAuthn: wa,
		Auth: api.AuthSettings{
			PublicURL:      cfg.PublicURL,
			CookieSecure:   cfg.CookieSecure,
			ClientIPHeader: cfg.ClientIPHeader,
			TrustedProxies: cfg.TrustedProxies,
		},
		Secrets: masterKey,
		Storage: objects,
		Attachments: api.AttachmentSettings{
			MaxBytes: cfg.Attachments.MaxBytes,
			Types:    cfg.Attachments.Types,
		},
		Hub:              hub,
		Ingress:          ingressSettings(cfg),
		Chat:             api.ChatSettings{EmailDelay: cfg.ChatEmailDelay, AnonymousContactsPerHour: cfg.AnonymousContactsPerHour},
		Webhooks:         api.WebhookSettings{AllowPrivate: cfg.WebhookAllowPrivate},
		SMTPAllowPrivate: cfg.SMTPAllowPrivate,
		Push:             api.PushSettings{Keys: pushKeys},
	})
	if cfg.SMTPAllowPrivate {
		log.Warn("YUVA_SMTP_ALLOW_PRIVATE is set; e-mail channels may send through SMTP servers on private and loopback addresses")
	}
	if cfg.WebhookAllowPrivate {
		log.Warn("YUVA_WEBHOOK_ALLOW_PRIVATE is set; webhooks may reach private and loopback addresses")
	}
	if cfg.IngressSecret == "" {
		log.Warn("YUVA_INGRESS_SECRET is not set; /ingress/email refuses all mail")
	}
	if cfg.IngressAcceptV1 {
		log.Warn("YUVA_INGRESS_ACCEPT_V1 is set; /ingress/email accepts v1 signatures, which do not cover the envelope sender")
	}
	if cfg.IngressSecret != "" && cfg.IngressAuthservID == "" {
		log.Warn("YUVA_INGRESS_AUTHSERV_ID is not set; DMARC results of inbound mail are stored but not used")
	}
	queue, err := jobs.New(st.Pool, st.Queries, log, srv.AddWorkers)
	if err != nil {
		return fmt.Errorf("job queue: %w", err)
	}
	if err := queue.Start(ctx); err != nil {
		return fmt.Errorf("job queue: %w", err)
	}
	defer func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		_ = queue.Stop(stopCtx)
	}()
	httpSrv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}
	go func() {
		<-ctx.Done()
		hub.StopAll(realtime.ReasonRestart)
		shutdown, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(shutdown)
	}()
	log.Info("listening", slog.String("addr", cfg.ListenAddr), slog.String("version", cfg.Version))
	if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func vapidKeys() error {
	public, private, err := push.GenerateKeys()
	if err != nil {
		return err
	}
	fmt.Printf("YUVA_VAPID_PUBLIC_KEY=%s\nYUVA_VAPID_PRIVATE_KEY=%s\n", public, private)
	return nil
}

func openStorage(c config.Storage) (storage.Storage, error) {
	if c.Driver == "s3" {
		return storage.NewS3(storage.S3Config{
			Endpoint: c.S3Endpoint, Region: c.S3Region, Bucket: c.S3Bucket,
			AccessKeyID: c.S3AccessKeyID, SecretAccessKey: c.S3SecretAccessKey, PathStyle: c.S3PathStyle,
		})
	}
	return storage.NewLocal(c.Dir)
}

func migrate(ctx context.Context, cfg config.Config, log *slog.Logger, args []string) error {
	if len(args) != 1 {
		return errors.New("usage: yuva migrate up|down|status")
	}
	st, err := openStore(ctx, cfg)
	if err != nil {
		return err
	}
	defer st.Close()
	switch args[0] {
	case "up":
		return migrateUp(ctx, st, log)
	case "down":
		return st.MigrateDown(ctx, log)
	case "status":
		statuses, err := st.MigrationStatus(ctx, log)
		if err != nil {
			return err
		}
		for _, s := range statuses {
			applied := "pending"
			if !s.AppliedAt.IsZero() {
				applied = s.AppliedAt.Format(time.RFC3339)
			}
			fmt.Printf("%05d  %-40s  %s\n", s.Source.Version, s.Source.Path, applied)
		}
		return nil
	default:
		return fmt.Errorf("unknown migrate command %q", args[0])
	}
}

func bootstrap(ctx context.Context, cfg config.Config, log *slog.Logger, args []string) error {
	fs := flag.NewFlagSet("bootstrap", flag.ContinueOnError)
	var in api.BootstrapInput
	fs.StringVar(&in.Email, "email", "", "e-mail address of the first owner")
	fs.StringVar(&in.Workspace, "workspace", "", "name of the workspace")
	fs.StringVar(&in.Name, "name", "", "display name of the owner")
	fs.StringVar(&in.Locale, "locale", "en", "language of e-mails to the owner: en or tr")
	fs.BoolVar(&in.AllowExisting, "allow-existing", false, "create a workspace even if one exists")
	if err := fs.Parse(args); err != nil {
		return err
	}
	st, err := openStore(ctx, cfg)
	if err != nil {
		return err
	}
	defer st.Close()
	if err := migrateUp(ctx, st, log); err != nil {
		return err
	}
	res, err := api.Bootstrap(ctx, st, in)
	if err != nil {
		return err
	}
	fmt.Printf("workspace %s\nmember    %s\nowner     %s\n", res.WorkspaceID, res.MemberID, in.Email)
	return nil
}

func apiKeyCLI(ctx context.Context, cfg config.Config, args []string) error {
	st, err := openStore(ctx, cfg)
	if err != nil {
		return err
	}
	defer st.Close()
	return apiKey(ctx, st, args, os.Stdout, os.Stderr)
}

const (
	exitDataErr  = 65
	exitNoUser   = 67
	exitTempFail = 75
	exitNoPerm   = 77
)

// ingestEmail is for MTAs that pipe a message into a command; the exit codes follow sysexits.h,
// which MTAs map to permanent or temporary failures.
func ingressSettings(cfg config.Config) api.IngressSettings {
	out := api.IngressSettings{
		Secret: cfg.IngressSecret, SESTopicARNs: cfg.SESTopicARNs, SenderHourlyCap: cfg.EmailSenderHourlyCap,
		AcceptV1: cfg.IngressAcceptV1, AuthservID: cfg.IngressAuthservID, MaxConcurrent: cfg.IngressMaxConcurrent,
	}
	if a, err := netmail.ParseAddress(cfg.SMTP.From); err == nil {
		out.OwnAddresses = []string{strings.ToLower(a.Address)}
	}
	return out
}

func ingestEmail(ctx context.Context, cfg config.Config, log *slog.Logger, args []string) int {
	fs := flag.NewFlagSet("ingest-email", flag.ContinueOnError)
	to := fs.String("to", "", "envelope recipient")
	from := fs.String("from", "", "envelope sender")
	if err := fs.Parse(args); err != nil || *to == "" {
		fmt.Fprintln(os.Stderr, "usage: yuva ingest-email --to <address> [--from <address>] < message.eml")
		return 64
	}
	raw, err := io.ReadAll(io.LimitReader(os.Stdin, api.MaxIngressBytes+1))
	if err != nil {
		fmt.Fprintln(os.Stderr, "read message:", err)
		return exitTempFail
	}
	if err := cfg.RequireMasterKey(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return exitTempFail
	}
	masterKey, err := secret.ParseKey(cfg.MasterKey)
	if err != nil {
		fmt.Fprintln(os.Stderr, "YUVA_MASTER_KEY:", err)
		return exitTempFail
	}
	objects, err := openStorage(cfg.Storage)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return exitTempFail
	}
	st, err := openStore(ctx, cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return exitTempFail
	}
	defer st.Close()
	srv := api.New(api.Deps{
		Log: log, Store: st, Version: cfg.Version, Mailer: mail.Log{Log: log}, Secrets: masterKey, Storage: objects,
		Attachments: api.AttachmentSettings{MaxBytes: cfg.Attachments.MaxBytes, Types: cfg.Attachments.Types},
		Ingress:     ingressSettings(cfg),
	})
	res, err := srv.IngestEmail(ctx, *to, *from, true, raw)
	var ie *api.IngestError
	if errors.As(err, &ie) {
		fmt.Fprintln(os.Stderr, ie.Reason)
		switch {
		case ie.Status >= 500:
			return exitTempFail
		case ie.Status == http.StatusNotFound:
			return exitNoUser
		case ie.Status == http.StatusBadRequest || ie.Status == http.StatusRequestEntityTooLarge:
			return exitDataErr
		default:
			return exitNoPerm
		}
	}
	if err != nil {
		log.Error("ingest email", slog.Any("error", err))
		fmt.Fprintln(os.Stderr, "Temporary failure, try again later")
		return exitTempFail
	}
	fmt.Println(res.Status)
	return 0
}
