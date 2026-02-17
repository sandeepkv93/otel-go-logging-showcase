package main

import (
    "context"
    "log/slog"
    "os"
    "os/signal"
    "syscall"
    "time"

    "github.com/sandeepkv93/otel-go-logging-showcase/pkg/config"
    "github.com/sandeepkv93/otel-go-logging-showcase/pkg/logging"
    "github.com/sandeepkv93/otel-go-logging-showcase/pkg/tracing"

    "go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/attribute"
    sdkresource "go.opentelemetry.io/otel/sdk/resource"
)

func main() {
    cfg := config.Load()
    ctx := context.Background()

    res, err := sdkresource.New(ctx,
        sdkresource.WithAttributes(
            attribute.String("service.name", cfg.ServiceName),
            attribute.String("deployment.environment", cfg.Environment),
        ),
    )
    if err != nil {
        panic(err)
    }

    var lp *logging.LoggerProvider
    if cfg.LogsEnabled {
        lp, err = logging.InitLogProvider(ctx, cfg, res)
        if err != nil {
            panic(err)
        }
    }

    tp, err := tracing.InitTracerProvider(ctx, cfg, res)
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

    logger := logging.InitLogger(cfg, lp)
    slog.SetDefault(logger)

    tracer := otel.Tracer("showcase")
    ctx, span := tracer.Start(context.Background(), "demo-operation")
    logger.InfoContext(ctx, "processing request", "route", "/demo", "method", "GET")
    logger.WarnContext(ctx, "slow dependency", "dependency", "redis", "latency_ms", 187)
    span.End()

    logger.Error("job failed", "job", "nightly-import", "retryable", true)
    logger.Info("waiting for ctrl+c to exit and flush", "endpoint", cfg.Endpoint)

    sig := make(chan os.Signal, 1)
    signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
    <-sig
    logger.Info("shutdown requested")
}
