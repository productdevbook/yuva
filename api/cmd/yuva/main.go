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

	"github.com/productdevbook/yuva/api/internal/api"
	"github.com/productdevbook/yuva/api/internal/config"
	"github.com/productdevbook/yuva/api/internal/jobs"
	"github.com/productdevbook/yuva/api/internal/metrics"
	"github.com/productdevbook/yuva/api/internal/store"
)

var version = ""

const usageText = `usage:
  yuva serve [--migrate=false]   (applies database migrations first unless disabled)
  yuva migrate up|down|status`

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
	if _, err := jobs.New(st.Pool, log); err != nil {
		return fmt.Errorf("job queue: %w", err)
	}
	if cfg.MetricsAddr != "" {
		go metrics.Serve(ctx, cfg.MetricsAddr, log)
	}
	srv := api.New(api.Deps{Log: log, Store: st, Version: cfg.Version})
	httpSrv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}
	go func() {
		<-ctx.Done()
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
