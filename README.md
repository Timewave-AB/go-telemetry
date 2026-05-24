# go-telemetry

Opinionated OpenTelemetry bootstrap for Go services. One `Init` call wires
logs, traces, and metrics against the same OTLP collector and returns a
bundle of handles.

Built and maintained by [Timewave](https://timewave.se).

## Install

```sh
go get github.com/Timewave-AB/go-telemetry
```

## Usage

### Bootstrap

```go
import (
    "context"

    "github.com/Timewave-AB/go-telemetry"
)

func main() {
    ctx := context.Background()
    tel, err := telemetry.Init(ctx, telemetry.Options{
        Level:          "info",
        OTLPEndpoint:   "otel-collector:4317", // empty disables OTLP entirely
        ServiceName:    "queue-worker",
        ServiceVersion: "", // empty → debug.ReadBuildInfo()
        Transport:      telemetry.TransportGRPC, // default; or TransportHTTP
    })
    if err != nil {
        panic(err)
    }
    defer tel.Shutdown(ctx)

    tel.Logger.Info("started", "workers", 4)
}
```

### Spans, with a logger that auto-correlates

`tel.Tracer.Start` returns the standard `(ctx, span)` plus a `*Logger`
pre-bound to that span's context. Plain `log.Info(...)` calls through the
returned logger are tagged with the span's `trace_id` and `span_id` — no
need to thread the context through every log call.

```go
func handleLogin(ctx context.Context, tel *telemetry.Telemetry, username string) error {
    ctx, span, log := tel.Tracer.Start(ctx, "login")
    defer span.End()

    log = log.With("username", username)
    log.Info("user is trying to login")

    if err := authenticate(ctx, username); err != nil {
        span.RecordError(err)
        log.Error("login failed", "err", err)
        return err
    }

    log.Info("login succeeded")
    return nil
}
```

Nested spans work the same way — pass the returned `ctx` to the next
`Start`, and each per-span logger correlates to its own span:

```go
ctx, outer, log := tel.Tracer.Start(ctx, "request")
defer outer.End()
log.Info("received")

ctx, inner, log := tel.Tracer.Start(ctx, "db.query")
defer inner.End()
log.Info("querying")  // tagged with the inner span
```

### Metrics

```go
jobs, _ := tel.Meter.Int64Counter("jobs.processed")
jobs.Add(ctx, 1)
```

`tel.Meter` is the standard OpenTelemetry `metric.Meter` — no wrapper.

### Configuration

| Field                  | Required | Default       | Notes                                                  |
|------------------------|----------|---------------|--------------------------------------------------------|
| `ServiceName`          | yes      | —             | Emitted as `service.name` on every signal.             |
| `ServiceVersion`       | no       | `ReadBuildInfo` → `"unknown"` | Emitted as `service.version`.        |
| `OTLPEndpoint`         | no       | `""`          | `""` disables OTLP entirely (stdout-only logs, noop traces/metrics). |
| `Transport`            | no       | `TransportGRPC` | `TransportGRPC` (port 4317) or `TransportHTTP` (port 4318). |
| `OTLPSecure`           | no       | `false`       | Set `true` to use TLS to the collector.                |
| `Level`                | no       | `info`        | `error`/`warn`/`info`/`verbose`/`debug` (case-insensitive). |
| `TraceSampleRatio`     | no       | `0` (always)  | `0` → always sample; `(0,1]` → ratio-based sampling.   |
| `MetricExportInterval` | no       | SDK default (60s) | How often metrics are pushed.                      |

If `OTLPEndpoint` has no port, the default port for the chosen transport
is filled in automatically.

## Log format

Stdout always receives plain text, even with OTLP enabled:

```
2026-05-24 14:02:11 [INFO] msg="started" workers=4
```

- Timestamps are UTC.
- Levels render as `ERROR`, `WARN`, `INFO`, `VERBOSE`, `DEBUG`.
- Attributes are sorted alphabetically per record.
- Groups become dotted prefixes: `req.id="abc"`.
- The format is fixed by design — every service using this package looks the
  same in `kubectl logs`/`docker logs`.

## Log levels

Five levels in slog's "higher value = more important" ordering:

| Name      | slog.Level |
|-----------|-----------:|
| `error`   | 8          |
| `warning` | 4          |
| `info`    | 0          |
| `verbose` | -2         |
| `debug`   | -4         |

Empty `Level` defaults to `info` silently; an unknown value emits one
stderr warning and falls back to `info`.

## No OTel globals

`Init` never calls `otel.SetTracerProvider`, `SetMeterProvider`,
`SetLoggerProvider`, or `SetTextMapPropagator`. Multiple `Init` calls in
the same process are independent.

The bundle's `LoggerProvider`, `TracerProvider`, `MeterProvider`, and
`Propagator` fields are an escape hatch for wiring third-party
instrumentation libraries that ask for them:

```go
import "go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

client := &http.Client{
    Transport: otelhttp.NewTransport(http.DefaultTransport,
        otelhttp.WithTracerProvider(tel.TracerProvider),
        otelhttp.WithPropagators(tel.Propagator)),
}
```

You can ignore these fields entirely if you don't need them.

### Last-resort global registration

If you discover a dependency that reads `otel.GetTracerProvider()`
unconditionally and silently drops spans, opt in at your `main`:

```go
otel.SetTracerProvider(tel.TracerProvider)
otel.SetTextMapPropagator(tel.Propagator)
```

Never call these inside library code — they belong in `main`.

### Reaching the underlying handles

- `tel.Logger.Slog()` returns the underlying `*slog.Logger` for libraries
  that take a stdlib logger directly.
- `tel.Tracer.OTel()` returns the underlying `trace.Tracer`.

## Behavioural contract

- `OTLPEndpoint == ""`: logs print to stdout; traces/metrics use noop
  providers. No exporters are created, no goroutines started.
- `OTLPEndpoint != ""`: logs fan out to stdout **and** OTLP; an OTLP
  failure on one record does not suppress stdout for that record.
- `tel.Shutdown(ctx)` runs under a 5-second timeout; calling it twice is
  a no-op.

## Testing

```sh
go test ./...
go test -race ./...
go vet ./...
```
