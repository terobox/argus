// Command server runs the argus central server: it ingests agent reports,
// serves the admin API, and (in later phases) the web UI and query API.
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
	"syscall"
	"time"

	"github.com/terobox/argus/pkg/alert"
	"github.com/terobox/argus/pkg/auth"
	"github.com/terobox/argus/pkg/config"
	"github.com/terobox/argus/pkg/server"
	"github.com/terobox/argus/pkg/storage"
)

// serverVersion is stamped at build time via
// -ldflags "-X main.serverVersion=v1.2.3".
var serverVersion = "dev"

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))
	if err := run(os.Args[1:]); err != nil {
		slog.Error("server exited", "err", err)
		os.Exit(1)
	}
}

// run dispatches the "version" subcommand and otherwise runs the server with
// the standard -config flag.
func run(args []string) error {
	if len(args) > 0 && args[0] == "version" {
		fmt.Println(serverVersion)
		return nil
	}

	fs := flag.NewFlagSet("argus-server", flag.ExitOnError)
	configPath := fs.String("config", "server.yaml", "path to the server config file")
	fs.Parse(args)
	return runServer(*configPath)
}

func runServer(configPath string) error {
	cfg, err := config.LoadServer(configPath)
	if err != nil {
		return err
	}

	store, err := storage.Open(cfg.DBPath)
	if err != nil {
		return err
	}
	defer store.Close()
	if err := store.Migrate(context.Background()); err != nil {
		return err
	}

	// Bootstrap the initial admin on first start, from the environment.
	username := os.Getenv("ARGUS_ADMIN_USERNAME")
	if username == "" {
		username = "admin"
	}
	created, err := auth.Bootstrap(context.Background(), store, username, os.Getenv("ARGUS_ADMIN_PASSWORD"))
	if err != nil {
		return err
	}
	if created {
		slog.Info("created initial admin", "username", username)
	}
	if n, err := store.CountAdmins(context.Background()); err != nil {
		return err
	} else if n == 0 {
		slog.Warn("no admin configured; set ARGUS_ADMIN_PASSWORD to create one on next start")
	}

	authSvc := auth.New(store, slog.Default(), cfg.CookieSecure, cfg.SessionTTL.D())
	srv := server.New(store, slog.Default(), authSvc, buildAlerts(store, cfg.Alert, slog.Default()))
	hs := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go srv.RunJobs(ctx, cfg.JobInterval.D())

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = hs.Shutdown(shutdownCtx)
	}()

	slog.Info("server listening", "addr", cfg.ListenAddr, "db", cfg.DBPath, "version", serverVersion)
	if err := hs.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// buildAlerts returns an alert evaluator when alerting is enabled, or nil to
// disable it. The runtime alert.Config is assembled from the on-disk form, whose
// defaults have already been applied by LoadServer.
func buildAlerts(store *storage.Store, ac config.AlertConfig, logger *slog.Logger) *alert.Evaluator {
	if !ac.Enabled {
		return nil
	}
	return alert.NewEvaluator(store, alert.NewDispatcher(ac.WebhookURL), alert.Config{
		Enabled:       true,
		WebhookURL:    ac.WebhookURL,
		OfflineAfter:  ac.OfflineAfter.D(),
		Sustain:       ac.Sustain.D(),
		CPUThreshold:  ac.CPUThreshold,
		MemThreshold:  ac.MemThreshold,
		DiskThreshold: ac.DiskThreshold,
		Cooldown:      ac.Cooldown.D(),
	}, logger)
}
