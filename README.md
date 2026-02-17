# otel-go-logging-showcase

Minimal Go repo that demonstrates how logs are emitted via OpenTelemetry using:
- `slog` for app logging
- `otelslog` bridge to transform `slog` records into OTel log records
- OTLP gRPC exporter (`otlploggrpc`) to send logs to OpenTelemetry Collector
- Collector pipeline to Loki

## How emission works

1. App writes logs with `slog`.
2. Logger uses `multiHandler` fan-out:
   - JSON logs to stdout
   - `otelslog` handler to OTel SDK log provider
3. OTel SDK log provider batches records and exports via OTLP gRPC to collector (`:4317`).
4. Collector logs pipeline exports to Loki (`/otlp`).
5. Grafana queries Loki and shows the log records.

`traceContextHandler` also injects `trace_id` and `span_id` into every log line so logs can be correlated with spans.

## Run

Start observability stack:

```bash
docker compose up -d
```

Run app:

```bash
go run .
```

Generate output by waiting for startup logs. Press `Ctrl+C` to flush and exit.

## View logs

1. Open `http://localhost:3000`.
2. Login with `admin/admin`.
3. Add Loki data source: URL `http://loki:3100`.
4. Go to Explore and query:

```logql
{service_name="otel-go-logging-showcase"}
```

## Useful env vars

- `OTEL_SERVICE_NAME` (default: `otel-go-logging-showcase`)
- `OTEL_ENVIRONMENT` (default: `development`)
- `OTEL_EXPORTER_OTLP_ENDPOINT` (default: `localhost:4317`)
- `OTEL_EXPORTER_OTLP_INSECURE` (default: `true`)
- `OTEL_LOGS_ENABLED` (default: `true`)
- `OTEL_TRACING_ENABLED` (default: `true`)
- `OTEL_LOG_LEVEL` (default: `info`)
