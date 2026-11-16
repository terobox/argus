// Command agent collects host metrics and pushes them to an argus server.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/terobox/argus/agent/buffer"
	"github.com/terobox/argus/agent/collect"
	"github.com/terobox/argus/agent/config"
	"github.com/terobox/argus/agent/push"
	"github.com/terobox/argus/pkg/protocol"
)

// agentVersion is stamped at build time via
// -ldflags "-X main.agentVersion=v1.2.3".
var agentVersion = "dev"

const (
	pushAttempts = 3
	pushBackoff  = time.Second
	flushTimeout = 5 * time.Second

	// hostInfoInterval is how often the agent re-reports its mostly-static host
	// info (in case the host, or its manually configured public IP, changes).
	hostInfoInterval = 30 * time.Minute
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))
	if err := run(os.Args[1:]); err != nil {
		slog.Error("agent exited", "err", err)
		os.Exit(1)
	}
}

// run dispatches subcommands ("update", "version") and otherwise runs the
// collector with the standard -config flag.
func run(args []string) error {
	if len(args) > 0 {
		switch args[0] {
		case "update":
			return runUpdate()
		case "version":
			fmt.Println(agentVersion)
			return nil
		}
	}

	fs := flag.NewFlagSet("argus-agent", flag.ExitOnError)
	configPath := fs.String("config", "agent/config.local.yaml", "path to the agent config file")
	fs.Parse(args)
	return runCollect(*configPath)
}

func runCollect(configPath string) error {
	cfg, err := config.LoadAgent(configPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("%w\nhint: copy agent/config.example.yaml to agent/config.local.yaml and pass -config <path>", err)
		}
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	sampler := collect.NewSampler(agentVersion)
	buf := buffer.New(cfg.BufferCapacity)
	client := push.New(cfg.ServerURL, cfg.Token, cfg.Gzip)

	slog.Info("agent started",
		"node_id", cfg.NodeID,
		"server", cfg.ServerURL,
		"collect", cfg.CollectInterval.D(),
		"push", cfg.PushInterval.D(),
	)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		collectLoop(ctx, cfg, sampler, buf)
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		pushLoop(ctx, cfg, buf, client)
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		hostLoop(ctx, cfg, sampler, client)
	}()

	<-ctx.Done()
	slog.Info("shutting down")
	wg.Wait()

	// Final flush so buffered samples survive a graceful stop. The server
	// ingests idempotently, so a sample the push loop already sent is simply
	// re-applied.
	flushCtx, cancel := context.WithTimeout(context.Background(), flushTimeout)
	defer cancel()
	flush(flushCtx, cfg.NodeID, buf, client)
	return nil
}

func collectLoop(ctx context.Context, cfg config.AgentConfig, sampler *collect.Sampler, buf *buffer.Buffer) {
	t := time.NewTicker(cfg.CollectInterval.D())
	defer t.Stop()

	var lastTS int64
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s, err := sampler.Sample(cfg.SlowInterval.D())
			if err != nil {
				slog.Warn("collect failed", "err", err)
				continue
			}
			// Enforce a strictly increasing timestamp so a report's samples
			// always validate and DropBefore can remove by TS unambiguously.
			now := time.Now().Unix()
			if now <= lastTS {
				now = lastTS + 1
			}
			lastTS = now
			s.TS = now
			buf.Push(s)
		}
	}
}

func pushLoop(ctx context.Context, cfg config.AgentConfig, buf *buffer.Buffer, client *push.Client) {
	t := time.NewTicker(cfg.PushInterval.D())
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			flush(ctx, cfg.NodeID, buf, client)
		}
	}
}

// hostLoop reports the host's identifying information once at startup and then
// periodically, so the dashboard always has an up-to-date picture even if the
// host (or its manually configured public IP) changes.
func hostLoop(ctx context.Context, cfg config.AgentConfig, sampler *collect.Sampler, client *push.Client) {
	send := func() {
		hi, err := sampler.HostInfo(cfg.PublicIP)
		if err != nil {
			slog.Warn("host info collect failed", "err", err)
			return
		}
		rep := &protocol.HostReport{
			Version:  protocol.Version,
			NodeID:   cfg.NodeID,
			AgentVer: agentVersion,
			Info:     hi,
		}
		if err := client.SendHost(ctx, rep); err != nil {
			slog.Warn("host info push failed", "err", err)
		}
	}

	send()
	t := time.NewTicker(hostInfoInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			send()
		}
	}
}

func flush(ctx context.Context, nodeID string, buf *buffer.Buffer, client *push.Client) {
	samples := buf.Peek()
	if len(samples) == 0 {
		return
	}
	report := &protocol.Report{
		Version:     protocol.Version,
		NodeID:      nodeID,
		AgentVer:    agentVersion,
		CollectedAt: samples[len(samples)-1].TS,
		Samples:     samples,
	}
	if err := client.SendWithRetry(ctx, report, pushAttempts, pushBackoff); err != nil {
		slog.Warn("push failed; samples kept for retry", "err", err, "count", len(samples))
		return
	}
	buf.DropBefore(samples[len(samples)-1].TS)
}
