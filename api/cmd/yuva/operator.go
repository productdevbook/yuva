package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/analytics"
	"github.com/productdevbook/yuva/api/internal/api"
	"github.com/productdevbook/yuva/api/internal/config"
	"github.com/productdevbook/yuva/api/internal/mail"
	"github.com/productdevbook/yuva/api/internal/oas"
	"github.com/productdevbook/yuva/api/internal/secret"
	"github.com/productdevbook/yuva/api/internal/store"
)

const (
	inboxUsage = "usage: yuva inbox create --workspace <id|name> --name <name> [--slug <slug>] [--locale <tag>] [--timezone <zone>] [--mode live|async] [--expected-reply-minutes <n>]\n" +
		"       yuva inbox list --workspace <id|name>\n" +
		"       yuva inbox ses-tenant show --workspace <id|name> [--inbox <id|slug>]\n" +
		"       yuva inbox ses-tenant sync (--workspace <id|name> [--inbox <id|slug>] | --all) [--dry-run]\n" +
		"       yuva inbox ses-tenant reinstate --workspace <id|name> --inbox <id|slug>"
	channelUsage = "usage: yuva channel create-email --workspace <id|name> --inbox <id|slug> --name <name> --address <address> [--display-name <name>] [--from-address <address>]\n" +
		"         [--smtp-host <host> [--smtp-port <n>] [--tls starttls|tls|none] [--smtp-username <name>] [--smtp-password-file <path|->]]\n" +
		"       yuva channel create-chat --workspace <id|name> --inbox <id|slug> --name <name> --origin <origin>... [--allow-anonymous] [--ask-email-offline=false] [--greeting <text>]\n" +
		"       yuva channel list --workspace <id|name> --inbox <id|slug>\n" +
		"       yuva channel prove --channel <id|address> [--workspace <id|name>] [--yes]\n" +
		"       yuva channel prove --all-unproven --workspace <id|name> [--yes]   (lists what it proves without --yes)"
	workspaceUsage = "usage: yuva workspace delete --workspace <id|name> --yes\n" +
		"       yuva workspace broadcasts approve|deny|suspend --workspace <id|name> [--reason <text>]\n" +
		"       yuva workspace broadcasts limit --workspace <id|name> --daily <n>   (0 removes the limit)\n" +
		"       yuva workspace broadcasts show --workspace <id|name>\n" +
		"       yuva workspace analytics-limit --workspace <id|name> <events|0>   (0 removes the limit)"
	analyticsUsage = "usage: yuva analytics property create --workspace <id|name> --name <name> --domain <host>... [--inbox <id|slug>] [--platform web|ios|android]... [--timezone <zone>]\n" +
		"       yuva analytics property list --workspace <id|name>\n" +
		"       yuva analytics geoip-update"
	personUsage = "usage: yuva person delete --email <address> --yes"
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
	srv := api.New(api.Deps{Log: log, Store: st, Version: cfg.Version, Mailer: mail.Log{Log: log, Full: cfg.MailLogFull}, Secrets: masterKey,
		Broadcasts: api.BroadcastSettings{Mode: cfg.Broadcasts, DailyRecipients: cfg.BroadcastDailyRecipients, LinkURL: cfg.MailLinkURL},
		Analytics:  analyticsSettings(cfg), SES: sesSettings(cfg), Auth: api.AuthSettings{PublicURL: cfg.PublicURL}})
	switch cmd {
	case "inbox":
		return inboxCommand(ctx, st, srv, args, os.Stdout, os.Stderr)
	case "workspace":
		if len(args) > 0 && args[0] == "broadcasts" {
			return broadcastsCommand(ctx, st, srv, args[1:], os.Stdout, os.Stderr)
		}
		if len(args) > 0 && args[0] == "analytics-limit" {
			return analyticsLimitCommand(ctx, st, srv, args[1:], os.Stderr)
		}
		return workspaceCommand(ctx, st, srv, args, os.Stderr)
	case "person":
		return personCommand(ctx, srv, args, os.Stderr)
	case "analytics":
		return analyticsCommand(ctx, st, srv, args, os.Stdout, os.Stderr)
	}
	return channelCommand(ctx, st, srv, args, os.Stdin, os.Stdout, os.Stderr)
}

func workspaceCommand(ctx context.Context, st *store.Store, srv *api.Server, args []string, stderr io.Writer) error {
	if len(args) == 0 || args[0] != "delete" {
		return errors.New(workspaceUsage)
	}
	fs := flag.NewFlagSet("workspace delete", flag.ContinueOnError)
	fs.SetOutput(stderr)
	workspace := fs.String("workspace", "", "workspace id or exact name")
	yes := fs.Bool("yes", false, "confirm that the workspace and everything in it is deleted")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New(workspaceUsage)
	}
	ws, err := api.FindWorkspace(ctx, st, *workspace)
	if err != nil {
		return err
	}
	if !*yes {
		return fmt.Errorf("this deletes workspace %q (%s) and everything in it; run again with --yes", ws.Name, ws.ID)
	}
	if err := srv.OperatorDeleteWorkspace(ctx, ws.ID); err != nil {
		return err
	}
	fmt.Fprintf(stderr, "workspace %q (%s) is closed; the server deletes its data in the background\n", ws.Name, ws.ID)
	return nil
}

func personCommand(ctx context.Context, srv *api.Server, args []string, stderr io.Writer) error {
	if len(args) == 0 || args[0] != "delete" {
		return errors.New(personUsage)
	}
	fs := flag.NewFlagSet("person delete", flag.ContinueOnError)
	fs.SetOutput(stderr)
	email := fs.String("email", "", "the person's e-mail address")
	yes := fs.Bool("yes", false, "confirm that the account is deleted")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if fs.NArg() != 0 || *email == "" {
		return errors.New(personUsage)
	}
	if !*yes {
		return fmt.Errorf("this deletes the account of %s with all their memberships; run again with --yes", *email)
	}
	p, err := srv.OperatorDeletePerson(ctx, *email)
	if err != nil {
		return err
	}
	fmt.Fprintf(stderr, "person %s (%s) deleted\n", p.Email, p.ID)
	return nil
}

func inboxCommand(ctx context.Context, st *store.Store, srv *api.Server, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return errors.New(inboxUsage)
	}
	if args[0] == "ses-tenant" {
		return sesTenantCommand(ctx, st, srv, args[1:], stdout, stderr)
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
	if args[0] == "prove" {
		return proveCommand(ctx, st, srv, args[1:], stdout, stderr)
	}
	fs := flag.NewFlagSet("channel "+args[0], flag.ContinueOnError)
	fs.SetOutput(stderr)
	workspace := fs.String("workspace", "", "workspace id or exact name")
	inbox := fs.String("inbox", "", "inbox id or slug")
	switch args[0] {
	case "create-email":
		name := fs.String("name", "", "name of the channel")
		address := fs.String("address", "", "address the channel receives mail at; *@domain receives every other address of the domain")
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
	case "create-chat":
		name := fs.String("name", "", "name of the channel")
		var origins repeated
		fs.Var(&origins, "origin", "an origin whose pages may use the widget, such as https://www.example.com (repeatable)")
		allowAnonymous := fs.Bool("allow-anonymous", false, "visitors without an identity token may chat")
		askEmailOffline := fs.Bool("ask-email-offline", true, "the widget asks for an e-mail address when nobody is available")
		greeting := fs.String("greeting", "", "shown before the first message; the inbox branding greeting when empty")
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
		c := &oas.ChatChannelInput{AllowedOrigins: make([]oas.Origin, len(origins))}
		for i, o := range origins {
			c.AllowedOrigins[i] = oas.Origin(o)
		}
		fs.Visit(func(f *flag.Flag) {
			switch f.Name {
			case "allow-anonymous":
				c.AllowAnonymous = allowAnonymous
			case "ask-email-offline":
				c.AskEmailOffline = askEmailOffline
			case "greeting":
				c.Greeting = greeting
			}
		})
		ch, err := srv.OperatorCreateChannel(ctx, ws.ID, in.Id, oas.ChannelCreate{Kind: oas.ChannelKindChat, Name: *name, Chat: c})
		if err != nil {
			return err
		}
		fmt.Fprintf(stderr, "chat channel %q for %s created in inbox %s\n", ch.Name, joinOrigins(ch.Chat.AllowedOrigins), in.Slug)
		_, err = fmt.Fprintf(stdout, "%s\t%s\n", ch.Id, ch.Chat.PublicKey)
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
		fmt.Fprintln(tw, "ID\tKIND\tNAME\tADDRESS\tSMTP\tPASSWORD\tPUBLIC KEY\tORIGINS")
		for _, c := range channels {
			address, smtp, password, publicKey, origins := "-", "-", "-", "-", "-"
			if c.Chat != nil {
				publicKey = c.Chat.PublicKey
				origins = joinOrigins(c.Chat.AllowedOrigins)
			}
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
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n", c.Id, c.Kind, c.Name, address, smtp, password, publicKey, origins)
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

func joinOrigins(origins []oas.Origin) string {
	s := make([]string, len(origins))
	for i, o := range origins {
		s[i] = string(o)
	}
	return strings.Join(s, ",")
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

func broadcastsCommand(ctx context.Context, st *store.Store, srv *api.Server, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return errors.New(workspaceUsage)
	}
	fs := flag.NewFlagSet("workspace broadcasts "+args[0], flag.ContinueOnError)
	fs.SetOutput(stderr)
	workspace := fs.String("workspace", "", "workspace id or exact name")
	reason := fs.String("reason", "", "the reason shown to the workspace")
	daily := fs.Int("daily", -1, "recipients a day; 0 removes the limit")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New(workspaceUsage)
	}
	ws, err := api.FindWorkspace(ctx, st, *workspace)
	if err != nil {
		return err
	}
	switch args[0] {
	case "approve":
		if err = srv.OperatorApproveBroadcasts(ctx, ws.ID, *reason); err == nil && srv.SESEnabled() {
			fmt.Fprintf(stderr, "workspace %q (%s): broadcasts approve done\n", ws.Name, ws.ID)
			if err := syncSESTenants(ctx, srv, ws, nil, false, stdout); err != nil {
				fmt.Fprintf(stderr, "%v; the approval stays\n", err)
			}
			return nil
		}
	case "deny":
		err = srv.OperatorDenyBroadcasts(ctx, ws.ID, *reason)
	case "suspend":
		err = srv.OperatorSuspendBroadcasts(ctx, ws.ID, *reason)
	case "limit":
		if *daily < 0 {
			return errors.New("limit needs --daily <n>")
		}
		err = srv.OperatorLimitBroadcasts(ctx, ws.ID, *daily)
	case "show":
		return showBroadcasts(ctx, srv, ws, stdout)
	default:
		return errors.New(workspaceUsage)
	}
	if err != nil {
		return err
	}
	fmt.Fprintf(stderr, "workspace %q (%s): broadcasts %s done\n", ws.Name, ws.ID, args[0])
	return nil
}

func showBroadcasts(ctx context.Context, srv *api.Server, ws store.Workspace, stdout io.Writer) error {
	sum, err := srv.OperatorBroadcastSummary(ctx, ws.ID)
	if err != nil {
		return err
	}
	limit := "none"
	if sum.DailyLimit >= 0 {
		limit = fmt.Sprint(sum.DailyLimit)
	}
	pct := func(n, of int64) string {
		if of == 0 {
			return "-"
		}
		return fmt.Sprintf("%.2f%%", 100*float64(n)/float64(of))
	}
	fmt.Fprintf(stdout, "workspace     %s (%s)\nstatus        %s", ws.Name, ws.ID, sum.Status)
	if sum.Reason != "" {
		fmt.Fprintf(stdout, " (%s)", sum.Reason)
	}
	fmt.Fprintf(stdout, "\ndaily limit   %s, sent today %d\nlast 30 days  %d sent, bounces %s, complaints %s\nsender domains %s\n",
		limit, sum.SentToday, sum.Sent30, pct(sum.Bounced30, sum.Sent30), pct(sum.Complained30, sum.Sent30), strings.Join(sum.SenderDomains, ", "))
	tw := tabwriter.NewWriter(stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "CREATED\tSTATE\tRECIPIENTS\tSENT\tBOUNCED\tCOMPLAINED\tNAME\tSUBJECT")
	for _, b := range sum.Latest {
		fmt.Fprintf(tw, "%s\t%s\t%d\t%d\t%d\t%d\t%s\t%s\n", b.CreatedAt.UTC().Format("2006-01-02 15:04"), b.State, b.Recipients, b.Sent,
			b.Bounced, b.Complained, b.Name, b.Subject)
	}
	return tw.Flush()
}

func analyticsLimitCommand(ctx context.Context, st *store.Store, srv *api.Server, args []string, stderr io.Writer) error {
	fs := flag.NewFlagSet("workspace analytics-limit", flag.ContinueOnError)
	fs.SetOutput(stderr)
	workspace := fs.String("workspace", "", "workspace id or exact name")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New(workspaceUsage)
	}
	events, err := strconv.ParseInt(fs.Arg(0), 10, 64)
	if err != nil || events < 0 {
		return errors.New("the limit must be a number of events, 0 for no limit")
	}
	ws, err := api.FindWorkspace(ctx, st, *workspace)
	if err != nil {
		return err
	}
	if err := srv.OperatorLimitAnalytics(ctx, ws.ID, events); err != nil {
		return err
	}
	if events == 0 {
		fmt.Fprintf(stderr, "workspace %q (%s): analytics events are not limited\n", ws.Name, ws.ID)
		return nil
	}
	fmt.Fprintf(stderr, "workspace %q (%s): at most %d analytics events a month\n", ws.Name, ws.ID, events)
	return nil
}

func analyticsCommand(ctx context.Context, st *store.Store, srv *api.Server, args []string, stdout, stderr io.Writer) error {
	if len(args) < 2 || args[0] != "property" {
		return errors.New(analyticsUsage)
	}
	fs := flag.NewFlagSet("analytics property "+args[1], flag.ContinueOnError)
	fs.SetOutput(stderr)
	workspace := fs.String("workspace", "", "workspace id or exact name")
	switch args[1] {
	case "create":
		name := fs.String("name", "", "name of the property")
		var domains, platforms repeated
		fs.Var(&domains, "domain", "a host name whose pages may send events, such as www.example.com or *.example.com (repeatable)")
		fs.Var(&platforms, "platform", "web, ios or android (repeatable; web when absent)")
		inbox := fs.String("inbox", "", "inbox id or slug whose agents may read the property")
		timezone := fs.String("timezone", "", "IANA time zone of the property's day; the inbox's, else UTC, when empty")
		if err := fs.Parse(args[2:]); err != nil {
			return err
		}
		if fs.NArg() != 0 {
			return errors.New(analyticsUsage)
		}
		ws, err := api.FindWorkspace(ctx, st, *workspace)
		if err != nil {
			return err
		}
		body := oas.AnalyticsPropertyCreate{Name: *name, Domains: domains}
		if *inbox != "" {
			_, in, err := findInbox(ctx, st, srv, ws.ID.String(), *inbox)
			if err != nil {
				return err
			}
			body.InboxId = &in.Id
		}
		if len(platforms) > 0 {
			ps := make([]oas.AnalyticsPlatform, len(platforms))
			for i, p := range platforms {
				ps[i] = oas.AnalyticsPlatform(p)
			}
			body.Platforms = &ps
		}
		if *timezone != "" {
			body.Timezone = timezone
		}
		p, err := srv.OperatorCreateAnalyticsProperty(ctx, ws.ID, body)
		if err != nil {
			return err
		}
		fmt.Fprintf(stderr, "analytics property %q for %s created in workspace %s\n", p.Name, strings.Join(p.Domains, ","), ws.ID)
		_, err = fmt.Fprintf(stdout, "%s\t%s\n", p.Id, p.PublicKey)
		return err
	case "list":
		if err := fs.Parse(args[2:]); err != nil {
			return err
		}
		if fs.NArg() != 0 {
			return errors.New(analyticsUsage)
		}
		ws, err := api.FindWorkspace(ctx, st, *workspace)
		if err != nil {
			return err
		}
		list, err := srv.OperatorListAnalyticsProperties(ctx, ws.ID)
		if err != nil {
			return err
		}
		tw := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "ID\tNAME\tPUBLIC KEY\tDOMAINS\tTIMEZONE\tCOLLECTION\tVISITORS TODAY")
		for _, p := range list.Items {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%d\n", p.Id, p.Name, p.PublicKey, strings.Join(p.Domains, ","), p.Timezone, p.Collection, p.VisitorsToday)
		}
		if err := tw.Flush(); err != nil {
			return err
		}
		limit := "none"
		if list.Usage.Limit != nil {
			limit = strconv.FormatInt(*list.Usage.Limit, 10)
		}
		_, err = fmt.Fprintf(stdout, "\nevents in %s: %d, limit %s\n", list.Usage.Month, list.Usage.Events, limit)
		return err
	default:
		return errors.New(analyticsUsage)
	}
}

// geoipUpdate downloads this month's DB-IP Lite database into YUVA_GEOIP_DIR unless it is there,
// or checks the operator's file with YUVA_GEOIP=file.
func geoipUpdate(ctx context.Context, cfg config.Config, args []string, stdout io.Writer) error {
	if len(args) != 0 {
		return errors.New(analyticsUsage)
	}
	a := cfg.Analytics
	if a.GeoIP == analytics.GeoOff {
		return errors.New("YUVA_GEOIP is off; set it to dbip (the default) or file")
	}
	g := &analytics.GeoIP{Mode: a.GeoIP, Dir: a.GeoIPDir, File: a.GeoIPFile}
	defer g.Close()
	path, err := g.Update(ctx)
	if err != nil {
		return err
	}
	info := g.Info()
	_, err = fmt.Fprintf(stdout, "%s\tbuilt %s\n", path, info.BuiltAt.Format(time.DateOnly))
	if err == nil && a.GeoIP == analytics.GeoDBIP {
		_, err = fmt.Fprintf(stdout, "%s (%s), CC BY 4.0\n", analytics.DBIPAttribution, analytics.DBIPAttributionURL)
	}
	return err
}

func sesTenantCommand(ctx context.Context, st *store.Store, srv *api.Server, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return errors.New(inboxUsage)
	}
	fs := flag.NewFlagSet("inbox ses-tenant "+args[0], flag.ContinueOnError)
	fs.SetOutput(stderr)
	workspace := fs.String("workspace", "", "workspace id or exact name")
	inbox := fs.String("inbox", "", "inbox id or slug")
	all := fs.Bool("all", false, "every inbox with an Amazon SES channel or tenant (sync)")
	dry := fs.Bool("dry-run", false, "print what sync would change in AWS and change nothing")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New(inboxUsage)
	}
	if !srv.SESEnabled() {
		return api.ErrSESNotConfigured
	}
	pick := func(required bool) (store.Workspace, *uuid.UUID, error) {
		if *inbox == "" && !required {
			ws, err := api.FindWorkspace(ctx, st, *workspace)
			return ws, nil, err
		}
		ws, in, err := findInbox(ctx, st, srv, *workspace, *inbox)
		if err != nil {
			return ws, nil, err
		}
		return ws, &in.Id, nil
	}
	switch args[0] {
	case "show":
		ws, inboxID, err := pick(false)
		if err != nil {
			return err
		}
		reports, err := srv.OperatorSESTenants(ctx, ws.ID, inboxID)
		if err != nil {
			return err
		}
		for _, r := range reports {
			printSESReport(stdout, r)
		}
		return nil
	case "sync":
		if *all {
			if *workspace != "" || *inbox != "" {
				return errors.New(inboxUsage)
			}
			refs, err := srv.OperatorSESInboxes(ctx)
			if err != nil {
				return err
			}
			failed := 0
			for _, r := range refs {
				res, err := srv.OperatorSyncSESTenant(ctx, r.WorkspaceID, r.InboxID, *dry)
				if !printSESSync(stdout, r.WorkspaceID.String(), r.InboxID.String(), res, err, *dry) {
					failed++
				}
			}
			if failed > 0 {
				return fmt.Errorf("%d of %d inboxes failed to sync", failed, len(refs))
			}
			return nil
		}
		ws, inboxID, err := pick(false)
		if err != nil {
			return err
		}
		return syncSESTenants(ctx, srv, ws, inboxID, *dry, stdout)
	case "reinstate":
		ws, inboxID, err := pick(true)
		if err != nil {
			return err
		}
		t, err := srv.OperatorReinstateSESTenant(ctx, ws.ID, *inboxID)
		if err != nil {
			return err
		}
		fmt.Fprintf(stdout, "tenant %s: sending status %s\n", t.Name, t.SendingStatus)
		return nil
	default:
		return errors.New(inboxUsage)
	}
}

func syncSESTenants(ctx context.Context, srv *api.Server, ws store.Workspace, inboxID *uuid.UUID, dry bool, stdout io.Writer) error {
	inboxes, err := srv.OperatorListInboxes(ctx, ws.ID)
	if err != nil {
		return err
	}
	failed := 0
	for _, in := range inboxes {
		if inboxID != nil && in.Id != *inboxID {
			continue
		}
		res, err := srv.OperatorSyncSESTenant(ctx, ws.ID, in.Id, dry)
		if !printSESSync(stdout, ws.Name, in.Slug, res, err, dry) {
			failed++
		}
	}
	if failed > 0 {
		return fmt.Errorf("%d inboxes of workspace %s failed to sync", failed, ws.ID)
	}
	return nil
}

func printSESSync(w io.Writer, workspace, inbox string, res api.SESSyncResult, err error, dry bool) bool {
	prefix := fmt.Sprintf("%s/%s", workspace, inbox)
	if err != nil {
		fmt.Fprintf(w, "%s: sync failed: %v\n", prefix, err)
		return false
	}
	tenant := res.Tenant
	if tenant == "" {
		tenant = "no tenant"
	}
	states := make([]string, 0, len(res.Channels))
	for _, c := range res.Channels {
		states = append(states, c.Address+" "+c.State)
	}
	fmt.Fprintf(w, "%s: %s; channels: %s\n", prefix, tenant, orDash(strings.Join(states, ", ")))
	verb := "done"
	if dry {
		verb = "would"
	}
	for _, a := range res.Actions {
		fmt.Fprintf(w, "  %s: %s\n", verb, a)
	}
	for _, c := range res.Channels {
		if c.Detail != "" {
			fmt.Fprintf(w, "  %s: %s\n", c.Address, c.Detail)
		}
	}
	return true
}

func printSESReport(w io.Writer, r api.SESInboxReport) {
	fmt.Fprintf(w, "inbox %s (%s, %s)\n", r.InboxName, r.InboxSlug, r.InboxID)
	if t := r.Tenant; t != nil {
		fmt.Fprintf(w, "  tenant          %s\n  arn             %s\n  state           %s", t.Name, orDash(deref(t.Arn)), t.State)
		if t.Error != nil {
			fmt.Fprintf(w, " (%s)", *t.Error)
		}
		fmt.Fprintf(w, "\n  sending status  %s (policy %s, aws %s)\n", t.SendingStatus, orDash(deref(t.CustomerStatus)), orDash(deref(t.AwsStatus)))
		if t.Cause != nil {
			fmt.Fprintf(w, "  cause           %s\n", *t.Cause)
		}
		if deref(t.AwsStatus) == "disabled" {
			fmt.Fprintln(w, "  Amazon SES itself paused this tenant; only an AWS Support case lifts it")
		}
		checked := "-"
		if t.CheckedAt != nil {
			checked = t.CheckedAt.UTC().Format(time.RFC3339)
		}
		fmt.Fprintf(w, "  impact          %s\n  checked at      %s\n", t.ReputationImpact, checked)
		for _, res := range r.Resources {
			fmt.Fprintf(w, "  associated      %s %s\n", res.Kind, res.Name)
		}
	} else {
		fmt.Fprintln(w, "  tenant          none")
	}
	for _, c := range r.Channels {
		if c.SES == nil {
			fmt.Fprintf(w, "  channel %s <%s>: no Amazon SES account\n", c.Name, c.Address)
			continue
		}
		fmt.Fprintf(w, "  channel %s <%s>: %s (%s)", c.Name, c.Address, c.SES.State, c.SES.Source)
		if c.SES.Detail != nil {
			fmt.Fprintf(w, ": %s", *c.SES.Detail)
		}
		fmt.Fprintln(w)
	}
}

func orDash(v string) string {
	if v == "" {
		return "-"
	}
	return v
}

func deref(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

// proveCommand lets the operator vouch for channel addresses they trust, under the rules of a
// proof: never a catch-all on a shared domain, never an address another workspace proved.
func proveCommand(ctx context.Context, st *store.Store, srv *api.Server, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("channel prove", flag.ContinueOnError)
	fs.SetOutput(stderr)
	workspace := fs.String("workspace", "", "workspace id or exact name")
	channel := fs.String("channel", "", "channel id or address")
	all := fs.Bool("all-unproven", false, "every unproven e-mail channel of --workspace")
	yes := fs.Bool("yes", false, "prove them; without it the command only lists them")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 || (*channel == "") == !*all || (*all && *workspace == "") {
		return errors.New(channelUsage)
	}
	var ws *uuid.UUID
	if *workspace != "" {
		w, err := api.FindWorkspace(ctx, st, *workspace)
		if err != nil {
			return err
		}
		ws = &w.ID
	}
	found, err := srv.OperatorEmailChannels(ctx, ws, *channel, *all)
	if err != nil {
		return err
	}
	if *channel != "" {
		switch {
		case len(found) == 0:
			return fmt.Errorf("no e-mail channel %s", *channel)
		case len(found) > 1:
			return fmt.Errorf("%d workspaces claim %s; name one with --workspace", len(found), *channel)
		}
	}
	tw := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "CHANNEL\tWORKSPACE\tADDRESS\tNAME\tRESULT")
	proved, refused, pending := 0, 0, 0
	for _, c := range found {
		result := "would prove"
		switch err := srv.OperatorProveCheck(ctx, c); {
		case c.Verified:
			result = "already proven"
		case err != nil:
			result, refused = "refused: "+err.Error(), refused+1
		case *yes:
			if err := srv.OperatorProveChannel(ctx, c); err != nil {
				result, refused = "refused: "+err.Error(), refused+1
			} else {
				result, proved = "proved", proved+1
			}
		default:
			pending++
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", c.ChannelID, c.WorkspaceID, c.Address, c.Name, result)
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	switch {
	case pending > 0:
		return errors.New("nothing was proven; run again with --yes to prove the channels listed as would prove")
	case refused > 0 && *channel != "":
		return errors.New("the channel was not proven")
	}
	fmt.Fprintf(stderr, "%d proven, %d refused\n", proved, refused)
	return nil
}
