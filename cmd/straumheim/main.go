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
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/deepskydatahq/straumheim/internal/buffer"
	"github.com/deepskydatahq/straumheim/internal/config"
	"github.com/deepskydatahq/straumheim/internal/input"
	"github.com/deepskydatahq/straumheim/internal/metrics"
	"github.com/deepskydatahq/straumheim/internal/pipeline"
	pubsubprofile "github.com/deepskydatahq/straumheim/internal/pubsub"
	"github.com/deepskydatahq/straumheim/internal/sink"
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

	switch cfg.Runtime.Mode {
	case "collector":
		return runCollector(cfg)
	case "writer":
		return runWriter(cfg)
	case "", "default":
		// Continue with the portable self-hosted memory pipeline.
	default:
		slog.Error("invalid runtime mode", "mode", cfg.Runtime.Mode)
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

	// Recovery middleware catches panics and returns 500.
	r.Use(middleware.Recoverer)

	// Structured request logging middleware.
	r.Use(requestLogger)

	// CORS middleware with configurable origins.
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins: cfg.Server.CORS.AllowedOrigins,
		AllowedMethods: []string{"GET", "POST", "OPTIONS"},
		AllowedHeaders: []string{"Content-Type"},
	}))

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

func runCollector(cfg *config.Config) int {
	if err := validateCollectorConfig(cfg); err != nil {
		slog.Error("invalid collector configuration", "error", err)
		return 1
	}
	ctx := context.Background()
	publisher, err := pubsubprofile.NewGooglePublisherPipeline(
		ctx,
		cfg.Runtime.PubSub.Project,
		cfg.Runtime.PubSub.Topic,
	)
	if err != nil {
		slog.Error("failed to initialize Pub/Sub publisher", "error", err)
		return 1
	}
	router := buildCollectorRouter(cfg, publisher)
	return serveRequestScoped(cfg, router, publisher.Close)
}

func runWriter(cfg *config.Config) int {
	if err := validateWriterConfig(cfg); err != nil {
		slog.Error("invalid writer configuration", "error", err)
		return 1
	}
	sinks, err := createSinks(cfg.Sinks)
	if err != nil {
		slog.Error("failed to create writer sink", "error", err)
		return 1
	}
	writer := sinks[0]
	if err := writer.Init(context.Background()); err != nil {
		slog.Error("failed to initialize writer sink", "error", err)
		if closeErr := writer.Close(); closeErr != nil {
			slog.Error("failed to close writer after initialization error", "error", closeErr)
		}
		return 1
	}
	router := buildWriterRouter(cfg, writer)
	return serveRequestScoped(cfg, router, writer.Close)
}

func validateCollectorConfig(cfg *config.Config) error {
	if cfg.Runtime.PubSub.Project == "" {
		return fmt.Errorf("runtime.pubsub.project is required")
	}
	if cfg.Runtime.PubSub.Topic == "" {
		return fmt.Errorf("runtime.pubsub.topic is required")
	}
	return nil
}

func validateWriterConfig(cfg *config.Config) error {
	if cfg.Runtime.PubSub.PushPath == "" || cfg.Runtime.PubSub.PushPath[0] != '/' {
		return fmt.Errorf("runtime.pubsub.push_path must start with /")
	}
	if len(cfg.Sinks) != 1 {
		return fmt.Errorf("writer mode requires exactly one sink")
	}
	if cfg.Sinks[0].Type != "bigquery" {
		return fmt.Errorf("writer mode requires a bigquery sink")
	}
	return nil
}

func buildCollectorRouter(cfg *config.Config, publisher pipeline.Pipeline) *chi.Mux {
	r := newRequestScopedRouter(cfg)
	registerInputs(r, cfg.Inputs, publisher)
	return r
}

func buildWriterRouter(cfg *config.Config, writer pubsubprofile.RecordWriter) *chi.Mux {
	r := newRequestScopedRouter(cfg)
	r.Post(cfg.Runtime.PubSub.PushPath, pubsubprofile.NewPushHandler(writer).ServeHTTP)
	return r
}

func newRequestScopedRouter(cfg *config.Config) *chi.Mux {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(requestLogger)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins: cfg.Server.CORS.AllowedOrigins,
		AllowedMethods: []string{"GET", "POST", "OPTIONS"},
		AllowedHeaders: []string{"Content-Type"},
	}))
	r.Get("/health", healthHandler)
	return r
}

func serveRequestScoped(cfg *config.Config, handler http.Handler, closeFn func() error) int {
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	srv := &http.Server{Addr: addr, Handler: handler}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		slog.Info("server listening", "addr", addr, "mode", cfg.Runtime.Mode)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case <-ctx.Done():
		slog.Info("shutdown signal received", "mode", cfg.Runtime.Mode)
	case err := <-errCh:
		if err != nil {
			slog.Error("server error", "error", err)
			_ = closeFn()
			return 1
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("server shutdown error", "error", err)
	}
	if err := closeFn(); err != nil {
		slog.Error("runtime close error", "error", err)
		return 1
	}
	slog.Info("shutdown complete", "mode", cfg.Runtime.Mode)
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
		case "clickhouse":
			if sc.Endpoint == "" {
				return nil, fmt.Errorf("clickhouse sink %q requires endpoint", sc.Name)
			}
			database := sc.Database
			if database == "" {
				database = "default"
			}
			table := sc.Table
			if table == "" {
				table = "events"
			}
			sinks = append(sinks, sink.NewClickHouseSink(sc.Endpoint, database, table, sc.Username, sc.Password))
		case "bigquery":
			if sc.Project == "" {
				return nil, fmt.Errorf("bigquery sink %q requires project", sc.Name)
			}
			if sc.Dataset == "" {
				return nil, fmt.Errorf("bigquery sink %q requires dataset", sc.Name)
			}
			if sc.Table == "" {
				return nil, fmt.Errorf("bigquery sink %q requires table", sc.Name)
			}
			if sc.Location == "" {
				return nil, fmt.Errorf("bigquery sink %q requires location", sc.Name)
			}
			sinks = append(sinks, sink.NewBigQuerySink(sink.BigQueryOptions{
				Project:             sc.Project,
				Dataset:             sc.Dataset,
				Table:               sc.Table,
				Location:            sc.Location,
				MaxInflightRequests: sc.MaxInflightRequests,
			}))
		case "file":
			if sc.OutputDir == "" {
				return nil, fmt.Errorf("file sink %q requires output_dir", sc.Name)
			}
			rotation := sc.RotationInterval
			if rotation == 0 {
				rotation = 5 * time.Minute
			}
			sinks = append(sinks, sink.NewFileSink(sc.OutputDir, rotation))
		default:
			return nil, fmt.Errorf("unknown sink type: %q", sc.Type)
		}
	}
	return sinks, nil
}

// statusWriter wraps http.ResponseWriter to capture the response status code.
type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

// requestLogger is middleware that logs each request with method, path, status,
// duration, and remote address. Requests to /health are skipped to reduce noise.
func requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			next.ServeHTTP(w, r)
			return
		}
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: 200}
		next.ServeHTTP(sw, r)
		slog.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", sw.status,
			"duration_ms", time.Since(start).Milliseconds(),
			"remote_addr", r.RemoteAddr,
		)
	})
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
		case "snowplow":
			cfg := input.SnowplowConfig{
				Enabled: ic.Enabled,
				Path:    ic.Path,
			}
			if ic.Snowplow != nil {
				cfg.Cookie = input.CookieConfig{
					Enabled: ic.Snowplow.Cookie.Enabled,
					Name:    ic.Snowplow.Cookie.Name,
					Domain:  ic.Snowplow.Cookie.Domain,
					TTL:     ic.Snowplow.Cookie.TTL,
				}
			}
			sp := input.NewSnowplowInput(cfg)
			sp.Register(r, p)
			slog.Info("registered input", "name", name, "protocol", sp.Protocol())
		case "pixel":
			px := input.NewPixel()
			px.Register(r, p)
			slog.Info("registered input", "name", name, "protocol", px.Protocol())
		default:
			slog.Warn("unknown input type, skipping", "name", name)
		}
	}
}
