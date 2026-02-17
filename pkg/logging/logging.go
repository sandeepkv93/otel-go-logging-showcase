package logging

import (
    "context"
    "fmt"
    "log/slog"
    "os"

    "github.com/sandeepkv93/otel-go-logging-showcase/pkg/config"

    "go.opentelemetry.io/contrib/bridges/otelslog"
    otlploggrpc "go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
    sdklog "go.opentelemetry.io/otel/sdk/log"
    sdkresource "go.opentelemetry.io/otel/sdk/resource"
    "go.opentelemetry.io/otel/trace"
)

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
    r.AddAttrs(
        slog.String("trace_id", traceID),
        slog.String("span_id", spanID),
    )
    return h.next.Handle(ctx, r)
}

func (h *traceContextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
    return &traceContextHandler{next: h.next.WithAttrs(attrs)}
}

func (h *traceContextHandler) WithGroup(name string) slog.Handler {
    return &traceContextHandler{next: h.next.WithGroup(name)}
}

func InitLogger(cfg config.Config, lp *sdklog.LoggerProvider) *slog.Logger {
    stdout := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel})
    if !cfg.LogsEnabled || lp == nil {
        return slog.New(&traceContextHandler{next: stdout})
    }
    otelHandler := otelslog.NewHandler(cfg.ServiceName, otelslog.WithLoggerProvider(lp))
    return slog.New(&traceContextHandler{next: &multiHandler{handlers: []slog.Handler{stdout, otelHandler}}})
}

func InitLogProvider(ctx context.Context, cfg config.Config, res *sdkresource.Resource) (*sdklog.LoggerProvider, error) {
    opts := []otlploggrpc.Option{otlploggrpc.WithEndpoint(cfg.Endpoint)}
    if cfg.Insecure {
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
