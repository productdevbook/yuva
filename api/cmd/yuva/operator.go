package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"text/tabwriter"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/api"
	"github.com/productdevbook/yuva/api/internal/config"
	"github.com/productdevbook/yuva/api/internal/mail"
	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/secret"
	"github.com/productdevbook/yuva/api/internal/store"
)

const (
	inboxUsage = "usage: yuva inbox create --workspace <id|name> --name <name> [--slug <slug>] [--locale <tag>] [--timezone <zone>] [--mode live|async] [--expected-reply-minutes <n>]\n" +
		"       yuva inbox list --workspace <id|name>"
	channelUsage = "usage: yuva channel create-email --workspace <id|name> --inbox <id|slug> --name <name> --address <address> [--display-name <name>] [--from-address <address>]\n" +
		"         [--smtp-host <host> [--smtp-port <n>] [--tls starttls|tls|none] [--smtp-username <name>] [--smtp-password-file <path|->]]\n" +
		"       yuva channel list --workspace <id|name> --inbox <id|slug>"
)

func operatorCLI(ctx context.Context, cfg config.Config, cmd string, args []string) error {
	if err := cfg.RequireMasterKey(); err != nil {
		return err
	}
	masterKey, err := secret.ParseKey(cfg.MasterKey)
	if err != nil {
		return fmt.Errorf("YUVA_MASTER_KEY: %w", err)
	}
	st, err := openStore(ctx, cfg)
	if err != nil {
		return err
	}
	defer st.Close()
	log := slog.Default()
	srv := api.New(api.Deps{Log: log, Store: st, Version: cfg.Version, Mailer: mail.Log{Log: log}, Secrets: masterKey})
	if cmd == "inbox" {
		return inboxCommand(ctx, st, srv, args, os.Stdout, os.Stderr)
	}
	return channelCommand(ctx, st, srv, args, os.Stdin, os.Stdout, os.Stderr)
}

func inboxCommand(ctx context.Context, st *store.Store, srv *api.Server, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return errors.New(inboxUsage)
	}
	fs := flag.NewFlagSet("inbox "+args[0], flag.ContinueOnError)
	fs.SetOutput(stderr)
	workspace := fs.String("workspace", "", "workspace id or exact name")
	switch args[0] {
	case "create":
		name := fs.String("name", "", "name of the inbox")
		slug := fs.String("slug", "", "slug; made from the name when empty")
		locale := fs.String("locale", "en", "default language tag")
		timezone := fs.String("timezone", "UTC", "IANA time zone")
		mode := fs.String("mode", string(oas.Async), "live or async")
		replyMinutes := fs.Int("expected-reply-minutes", 0, "expected reply time shown in async mode; 0 for none")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if fs.NArg() != 0 {
			return errors.New(inboxUsage)
		}
		ws, err := api.FindWorkspace(ctx, st, *workspace)
		if err != nil {
			return err
		}
		if *slug == "" {
			*slug = slugify(*name)
		}
		m := oas.InboxMode(*mode)
		body := oas.InboxCreate{Name: *name, Slug: *slug, DefaultLocale: locale, Timezone: timezone, Mode: &m}
		if *replyMinutes != 0 {
			v := int32(*replyMinutes)
			body.ExpectedReplyMinutes = &v
		}
		in, _, err := srv.OperatorCreateInbox(ctx, ws.ID, body)
		if err != nil {
			return err
		}
		fmt.Fprintf(stderr, "inbox %q (%s) created in workspace %s; rotate its identity secret in the panel when an app needs it\n", in.Name, in.Slug, ws.ID)
		_, err = fmt.Fprintln(stdout, in.Id)
		return err
	case "list":
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if fs.NArg() != 0 {
			return errors.New(inboxUsage)
		}
		ws, err := api.FindWorkspace(ctx, st, *workspace)
		if err != nil {
			return err
		}
		inboxes, err := srv.OperatorListInboxes(ctx, ws.ID)
		if err != nil {
			return err
		}
		tw := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "ID\tSLUG\tNAME\tLOCALE\tTIMEZONE\tMODE\tREPLY MINUTES")
		for _, in := range inboxes {
			reply := "-"
			if in.ExpectedReplyMinutes != nil {
				reply = fmt.Sprint(*in.ExpectedReplyMinutes)
			}
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", in.Id, in.Slug, in.Name, in.DefaultLocale, in.Timezone, in.Mode, reply)
		}
		return tw.Flush()
	default:
		return errors.New(inboxUsage)
	}
}

func channelCommand(ctx context.Context, st *store.Store, srv *api.Server, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return errors.New(channelUsage)
	}
	fs := flag.NewFlagSet("channel "+args[0], flag.ContinueOnError)
	fs.SetOutput(stderr)
	workspace := fs.String("workspace", "", "workspace id or exact name")
	inbox := fs.String("inbox", "", "inbox id or slug")
	switch args[0] {
	case "create-email":
		name := fs.String("name", "", "name of the channel")
		address := fs.String("address", "", "address the channel receives mail at")
		displayName := fs.String("display-name", "", "name in From; the channel name when empty")
		fromAddress := fs.String("from-address", "", "sending address when it differs from --address")
		smtpHost := fs.String("smtp-host", "", "outbound SMTP host")
		smtpPort := fs.Int("smtp-port", 0, "outbound SMTP port; 465 with --tls tls, else 587")
		tls := fs.String("tls", "", "starttls, tls or none; starttls when empty")
		smtpUsername := fs.String("smtp-username", "", "SMTP user name")
		passwordFile := fs.String("smtp-password-file", "", "file holding the SMTP password, - for stdin")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if fs.NArg() != 0 {
			return errors.New(channelUsage)
		}
		ws, in, err := findInbox(ctx, st, srv, *workspace, *inbox)
		if err != nil {
			return err
		}
		e := &oas.EmailChannelInput{Address: oas.Email(*address)}
		if *displayName != "" {
			e.DisplayName = displayName
		}
		if *fromAddress != "" {
			from := oas.Email(*fromAddress)
			e.FromAddress = &from
		}
		if *smtpHost == "" && (*smtpPort != 0 || *tls != "" || *smtpUsername != "" || *passwordFile != "") {
			return errors.New("--smtp-port, --tls, --smtp-username and --smtp-password-file need --smtp-host")
		}
		if *smtpHost != "" {
			sm := &oas.SmtpSettingsInput{Host: *smtpHost}
			if *smtpPort != 0 {
				p := int32(*smtpPort)
				sm.Port = &p
			}
			if *tls != "" {
				t := oas.SmtpTls(*tls)
				sm.Tls = &t
			}
			if *smtpUsername != "" {
				sm.Username = smtpUsername
			}
			if *passwordFile != "" {
				pw, err := readPassword(*passwordFile, stdin)
				if err != nil {
					return err
				}
				sm.Password = &pw
			}
			e.Smtp = sm
		}
		c, err := srv.OperatorCreateChannel(ctx, ws.ID, in.Id, oas.ChannelCreate{Kind: oas.ChannelKindEmail, Name: *name, Email: e})
		if err != nil {
			return err
		}
		fmt.Fprintf(stderr, "email channel %q for %s created in inbox %s\n", c.Name, c.Email.Address, in.Slug)
		_, err = fmt.Fprintln(stdout, c.Id)
		return err
	case "list":
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if fs.NArg() != 0 {
			return errors.New(channelUsage)
		}
		ws, in, err := findInbox(ctx, st, srv, *workspace, *inbox)
		if err != nil {
			return err
		}
		channels, err := srv.OperatorListChannels(ctx, ws.ID, in.Id)
		if err != nil {
			return err
		}
		tw := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "ID\tKIND\tNAME\tADDRESS\tSMTP\tPASSWORD")
		for _, c := range channels {
			address, smtp, password := "-", "-", "-"
			if c.Email != nil {
				address = string(c.Email.Address)
				if c.Email.Smtp != nil {
					smtp = fmt.Sprintf("%s:%d %s %s", c.Email.Smtp.Host, c.Email.Smtp.Port, c.Email.Smtp.Tls, c.Email.Smtp.Username)
					password = "not set"
					if c.Email.Smtp.PasswordSet {
						password = "set"
					}
				}
			}
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", c.Id, c.Kind, c.Name, address, smtp, password)
		}
		return tw.Flush()
	default:
		return errors.New(channelUsage)
	}
}

func findInbox(ctx context.Context, st *store.Store, srv *api.Server, workspace, ref string) (store.Workspace, oas.Inbox, error) {
	ws, err := api.FindWorkspace(ctx, st, workspace)
	if err != nil {
		return ws, oas.Inbox{}, err
	}
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ws, oas.Inbox{}, errors.New("--inbox is required")
	}
	inboxes, err := srv.OperatorListInboxes(ctx, ws.ID)
	if err != nil {
		return ws, oas.Inbox{}, err
	}
	id, idErr := uuid.Parse(ref)
	for _, in := range inboxes {
		if (idErr == nil && in.Id == id) || in.Slug == ref {
			return ws, in, nil
		}
	}
	return ws, oas.Inbox{}, fmt.Errorf("no inbox %q in workspace %s", ref, ws.ID)
}

func readPassword(path string, stdin io.Reader) (string, error) {
	var b []byte
	var err error
	if path == "-" {
		b, err = io.ReadAll(io.LimitReader(stdin, 4096))
	} else {
		b, err = os.ReadFile(path)
	}
	if err != nil {
		return "", fmt.Errorf("read SMTP password: %w", err)
	}
	pw := strings.TrimRight(string(b), "\r\n")
	if pw == "" {
		return "", errors.New("the SMTP password file is empty")
	}
	return pw, nil
}

func slugify(name string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(name) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			if dash && b.Len() > 0 {
				b.WriteByte('-')
			}
			b.WriteRune(r)
			dash = false
			continue
		}
		dash = true
	}
	return b.String()
}
