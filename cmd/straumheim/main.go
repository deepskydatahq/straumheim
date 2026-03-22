// Command straumheim is a lightweight, self-hosted event data pipeline.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/deepsky-data/straumheim/internal/buffer"
	"github.com/deepsky-data/straumheim/internal/config"
	"github.com/deepsky-data/straumheim/internal/input"
	"github.com/deepsky-data/straumheim/internal/metrics"
	"github.com/deepsky-data/straumheim/internal/pipeline"
	"github.com/deepsky-data/straumheim/internal/sink"
)

func main() {
	os.Exit(run())
}

func run() int {
	// Parse config path from flag or env var.
	configPath := flag.String("config", "", "path to config file")
	flag.Parse()

	if *configPath == "" {
		if envPath := os.Getenv("STRAUMHEIM_CONFIG"); envPath != "" {
			*configPath = envPath
		} else {
			*configPath = "config.yaml"
		}
	}

	// Load configuration.
	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		slog.Error("failed to load config", "path", *configPath, "error", err)
		return 1
	}

	// Create sinks from config.
	sinks, err := createSinks(cfg.Sinks)
	if err != nil {
		slog.Error("failed to create sinks", "error", err)
		return 1
	}

	// Initialize sinks.
	ctx := context.Background()
	for _, s := range sinks {
		if err := s.Init(ctx); err != nil {
			slog.Error("failed to initialize sink", "error", err)
			return 1
		}
	}

	// Create memory buffer.
	buf := buffer.NewMemoryBuffer(
		cfg.Buffer.Capacity,
		cfg.Buffer.FlushCount,
		cfg.Buffer.FlushInterval,
	)

	// Create Prometheus metrics with custom registry.
	promReg := prometheus.NewRegistry()
	met := metrics.NewMetrics(promReg)

	// Collect sink names for metric labels.
	sinkNames := make([]string, len(cfg.Sinks))
	for i, sc := range cfg.Sinks {
		sinkNames[i] = sc.Name
	}

	// Create pipeline engine.
	engine := pipeline.NewEngine(buf, sinks, sinkNames, met)

	// Set up Chi router.
	r := chi.NewRouter()

	// Health check endpoint.
	r.Get("/health", healthHandler)

	// Prometheus metrics endpoint.
	r.Get("/metrics", promhttp.HandlerFor(promReg, promhttp.HandlerOpts{}).ServeHTTP)

	// Register inputs.
	registerInputs(r, cfg.Inputs, engine)

	// Create HTTP server.
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	srv := &http.Server{
		Addr:    addr,
		Handler: r,
	}

	// Graceful shutdown with signal handling.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	// Start server in a goroutine.
	errCh := make(chan error, 1)
	go func() {
		slog.Info("server listening", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
		close(errCh)
	}()

	// Wait for shutdown signal or server error.
	select {
	case <-ctx.Done():
		slog.Info("shutdown signal received")
	case err := <-errCh:
		if err != nil {
			slog.Error("server error", "error", err)
			return 1
		}
	}

	// Graceful shutdown: stop accepting requests.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("server shutdown error", "error", err)
	}

	// Flush pipeline.
	slog.Info("flushing pipeline")
	if err := engine.Close(); err != nil {
		slog.Error("pipeline close error", "error", err)
		return 1
	}

	slog.Info("shutdown complete")
	return 0
}

func healthHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func createSinks(configs []config.SinkConfig) ([]sink.Sink, error) {
	var sinks []sink.Sink
	for _, sc := range configs {
		switch sc.Type {
		case "stdout":
			sinks = append(sinks, sink.NewStdoutSink(nil))
		case "postgres":
			if sc.DSN == "" {
				return nil, fmt.Errorf("postgres sink %q requires dsn", sc.Name)
			}
			sinks = append(sinks, sink.NewPostgresSink(sc.DSN))
		default:
			return nil, fmt.Errorf("unknown sink type: %q", sc.Type)
		}
	}
	return sinks, nil
}

func registerInputs(r chi.Router, inputs map[string]config.InputConfig, p pipeline.Pipeline) {
	for name, ic := range inputs {
		if !ic.Enabled {
			continue
		}
		switch name {
		case "webhook":
			wh := input.NewWebhook()
			wh.Register(r, p)
			slog.Info("registered input", "name", name, "protocol", wh.Protocol())
		default:
			slog.Warn("unknown input type, skipping", "name", name)
		}
	}
}
