package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"
	"time"
	_ "time/tzdata"

	"github.com/go-webauthn/webauthn/webauthn"

	"github.com/productdevbook/yuva/api/internal/api"
	"github.com/productdevbook/yuva/api/internal/config"
	"github.com/productdevbook/yuva/api/internal/jobs"
	"github.com/productdevbook/yuva/api/internal/mail"
	"github.com/productdevbook/yuva/api/internal/metrics"
	"github.com/productdevbook/yuva/api/internal/realtime"
	"github.com/productdevbook/yuva/api/internal/secret"
	"github.com/productdevbook/yuva/api/internal/storage"
	"github.com/productdevbook/yuva/api/internal/store"
)

var version = ""

const usageText = `usage:
  yuva serve [--migrate=false]   (applies database migrations first unless disabled)
  yuva migrate up|down|status
  yuva bootstrap --email <address> --workspace <name> [--name <name>] [--locale en|tr] [--allow-existing]`

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	slog.SetDefault(log)
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, usageText)
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	cfg, err := config.Load(buildVersion())
	if err == nil {
		switch os.Args[1] {
		case "serve":
			err = serve(ctx, cfg, log, os.Args[2:])
		case "migrate":
			err = migrate(ctx, cfg, log, os.Args[2:])
		case "bootstrap":
			err = bootstrap(ctx, cfg, log, os.Args[2:])
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
	queue, err := jobs.New(st.Pool, st.Queries, log)
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
		},
		Secrets: masterKey,
		Storage: objects,
		Attachments: api.AttachmentSettings{
			MaxBytes: cfg.Attachments.MaxBytes,
			Types:    cfg.Attachments.Types,
		},
		Hub: hub,
	})
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
