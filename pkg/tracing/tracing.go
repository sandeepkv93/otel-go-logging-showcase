package tracing

import (
    "context"
    "fmt"

    "github.com/sandeepkv93/otel-go-logging-showcase/pkg/config"

    "go.opentelemetry.io/otel"
    otlptracegrpc "go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
    sdkresource "go.opentelemetry.io/otel/sdk/resource"
    sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func InitTracerProvider(ctx context.Context, cfg config.Config, res *sdkresource.Resource) (*sdktrace.TracerProvider, error) {
    if !cfg.TracesEnabled {
        tp := sdktrace.NewTracerProvider(sdktrace.WithResource(res))
        otel.SetTracerProvider(tp)
        return tp, nil
    }

    opts := []otlptracegrpc.Option{otlptracegrpc.WithEndpoint(cfg.Endpoint)}
    if cfg.Insecure {
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
