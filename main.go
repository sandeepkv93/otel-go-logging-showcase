package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	otlploggrpc "go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

type config struct {
	serviceName  string
	environment  string
	endpoint     string
	insecure     bool
	logsEnabled  bool
	tracesEnable bool
	logLevel     slog.Level
}

type multiHandler struct {
	handlers []slog.Handler
}

func (h *multiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, handler := range h.handlers {
		if handler.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (h *multiHandler) Handle(ctx context.Context, r slog.Record) error {
	for _, handler := range h.handlers {
		if err := handler.Handle(ctx, r); err != nil {
			return err
		}
	}
	return nil
}

func (h *multiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := make([]slog.Handler, 0, len(h.handlers))
	for _, handler := range h.handlers {
		next = append(next, handler.WithAttrs(attrs))
	}
	return &multiHandler{handlers: next}
}

func (h *multiHandler) WithGroup(name string) slog.Handler {
	next := make([]slog.Handler, 0, len(h.handlers))
	for _, handler := range h.handlers {
		next = append(next, handler.WithGroup(name))
	}
	return &multiHandler{handlers: next}
}

type traceContextHandler struct {
	next slog.Handler
}

func (h *traceContextHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h *traceContextHandler) Handle(ctx context.Context, r slog.Record) error {
	traceID := ""
	spanID := ""
	sc := trace.SpanContextFromContext(ctx)
	if sc.IsValid() {
		traceID = sc.TraceID().String()
		spanID = sc.SpanID().String()
	}
	r.AddAttrs(slog.String("trace_id", traceID), slog.String("span_id", spanID))
	return h.next.Handle(ctx, r)
}

func (h *traceContextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &traceContextHandler{next: h.next.WithAttrs(attrs)}
}

func (h *traceContextHandler) WithGroup(name string) slog.Handler {
	return &traceContextHandler{next: h.next.WithGroup(name)}
}

func main() {
	cfg := loadConfig()
	ctx := context.Background()

	res, err := sdkresource.New(ctx,
		sdkresource.WithAttributes(
			attribute.String("service.name", cfg.serviceName),
			attribute.String("deployment.environment", cfg.environment),
		),
	)
	if err != nil {
		panic(fmt.Errorf("create resource: %w", err))
	}

	var lp *sdklog.LoggerProvider
	if cfg.logsEnabled {
		lp, err = initLogProvider(ctx, cfg, res)
		if err != nil {
			panic(err)
		}
	}

	tp, err := initTracerProvider(ctx, cfg, res)
	if err != nil {
		panic(err)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = tp.Shutdown(shutdownCtx)
		if lp != nil {
			_ = lp.Shutdown(shutdownCtx)
		}
	}()

	logger := initLogger(cfg, lp)
	slog.SetDefault(logger)

	tracer := otel.Tracer("showcase")
	ctx, span := tracer.Start(context.Background(), "demo-operation")
	logger.InfoContext(ctx, "processing request", "route", "/demo", "method", "GET")
	logger.WarnContext(ctx, "slow dependency", "dependency", "redis", "latency_ms", 187)
	span.End()

	logger.Error("job failed", "job", "nightly-import", "retryable", true)
	logger.Info("waiting for ctrl+c to exit and flush", "endpoint", cfg.endpoint)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	logger.Info("shutdown requested")
}

func initLogProvider(ctx context.Context, cfg config, res *sdkresource.Resource) (*sdklog.LoggerProvider, error) {
	opts := []otlploggrpc.Option{otlploggrpc.WithEndpoint(cfg.endpoint)}
	if cfg.insecure {
		opts = append(opts, otlploggrpc.WithInsecure())
	}
	exporter, err := otlploggrpc.New(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("create otlp log exporter: %w", err)
	}

	lp := sdklog.NewLoggerProvider(
		sdklog.WithResource(res),
		sdklog.WithProcessor(sdklog.NewBatchProcessor(exporter)),
	)
	return lp, nil
}

func initTracerProvider(ctx context.Context, cfg config, res *sdkresource.Resource) (*sdktrace.TracerProvider, error) {
	if !cfg.tracesEnable {
		tp := sdktrace.NewTracerProvider(sdktrace.WithResource(res))
		otel.SetTracerProvider(tp)
		return tp, nil
	}

	opts := []otlptracegrpc.Option{otlptracegrpc.WithEndpoint(cfg.endpoint)}
	if cfg.insecure {
		opts = append(opts, otlptracegrpc.WithInsecure())
	}
	exporter, err := otlptracegrpc.New(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("create otlp trace exporter: %w", err)
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	return tp, nil
}

func initLogger(cfg config, lp *sdklog.LoggerProvider) *slog.Logger {
	stdout := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.logLevel})
	if !cfg.logsEnabled || lp == nil {
		return slog.New(&traceContextHandler{next: stdout})
	}
	otelHandler := otelslog.NewHandler(cfg.serviceName, otelslog.WithLoggerProvider(lp))
	return slog.New(&traceContextHandler{next: &multiHandler{handlers: []slog.Handler{stdout, otelHandler}}})
}

func loadConfig() config {
	return config{
		serviceName:  env("OTEL_SERVICE_NAME", "otel-go-logging-showcase"),
		environment:  env("OTEL_ENVIRONMENT", "development"),
		endpoint:     env("OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:4317"),
		insecure:     envBool("OTEL_EXPORTER_OTLP_INSECURE", true),
		logsEnabled:  envBool("OTEL_LOGS_ENABLED", true),
		tracesEnable: envBool("OTEL_TRACING_ENABLED", true),
		logLevel:     parseLevel(env("OTEL_LOG_LEVEL", "info")),
	}
}

func parseLevel(v string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func env(k, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return fallback
}

func envBool(k string, fallback bool) bool {
	v, ok := os.LookupEnv(k)
	if !ok {
		return fallback
	}
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "t", "yes", "y", "on":
		return true
	case "0", "false", "f", "no", "n", "off":
		return false
	default:
		return fallback
	}
}
