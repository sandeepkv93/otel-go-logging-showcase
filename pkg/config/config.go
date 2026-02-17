package config

import (
    "log/slog"
    "os"
    "strings"
)

type Config struct {
    ServiceName  string
    Environment  string
    Endpoint     string
    Insecure     bool
    LogsEnabled  bool
    TracesEnabled bool
    LogLevel     slog.Level
}

func Load() Config {
    return Config{
        ServiceName:  env("OTEL_SERVICE_NAME", "otel-go-logging-showcase"),
        Environment:  env("OTEL_ENVIRONMENT", "development"),
        Endpoint:     env("OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:4317"),
        Insecure:     envBool("OTEL_EXPORTER_OTLP_INSECURE", true),
        LogsEnabled:  envBool("OTEL_LOGS_ENABLED", true),
        TracesEnabled: envBool("OTEL_TRACING_ENABLED", true),
        LogLevel:     parseLevel(env("OTEL_LOG_LEVEL", "info")),
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

func env(key, fallback string) string {
    if v := strings.TrimSpace(os.Getenv(key)); v != "" {
        return v
    }
    return fallback
}

func envBool(key string, fallback bool) bool {
    v, ok := os.LookupEnv(key)
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
