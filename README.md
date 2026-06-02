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

`tel.Tracer.Start` returns the child context and a `*SpanLogger` bound
to the new span. `log.Info(...)` calls through it are tagged with the
span's `trace_id` and `span_id` — no need to thread the context through
every log call. End the span via `log.Span().End()`.

```go
func handleLogin(ctx context.Context, tel *telemetry.Telemetry, username string) error {
    ctx, log := tel.Tracer.Start(ctx, "login")
    defer log.Span().End()

    log = log.With("username", username)
    log.Info("user is trying to login")

    if err := authenticate(ctx, username); err != nil {
        log.Span().RecordError(err)
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
ctx, log := tel.Tracer.Start(ctx, "request")
defer log.Span().End()
log.Info("received")

ctx, inner := tel.Tracer.Start(ctx, "db.query")
defer inner.Span().End()
inner.Info("querying")  // tagged with the inner span
```

The top-level `tel.Logger` is a separate `*Logger` type with no span
binding — it always emits with `context.Background()`. Use it for
process-wide messages (startup, shutdown, periodic stats) where there is
no active span.

### Joining an incoming trace

When a reverse-proxy (or any upstream) sends a W3C `traceparent`,
`tel.Tracer.Extract` reads it from the incoming carrier and returns a
context. The next `Start` then makes the service's span a child of the
upstream span, in the *same* trace — so the service shows up under the
proxy's request in Tempo/Grafana instead of in a detached trace. A
missing or invalid header falls back to a fresh trace; `Extract` never
errors.

```go
import (
    "net/http"

    "go.opentelemetry.io/otel/propagation"
    "go.opentelemetry.io/otel/trace"
)

func handler(w http.ResponseWriter, r *http.Request) {
    ctx := tel.Tracer.Extract(r.Context(), propagation.HeaderCarrier(r.Header))
    ctx, log := tel.Tracer.Start(ctx, "GET /thing", trace.WithSpanKind(trace.SpanKindServer))
    defer log.Span().End()
    log.Info("handling request")
}
```

`Extract` uses the composite propagator (`TraceContext` + `Baggage`) also
exposed at `tel.OTel().Propagator` — it just saves you wiring the
extraction by hand. It accepts any `propagation.TextMapCarrier`; wrap an
`http.Header` with `propagation.HeaderCarrier`, or a `map[string]string`
with `propagation.MapCarrier`.

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
| `LogExporter`          | no       | OTLP              | Override the log exporter (tests, non-OTLP backends). |
| `TraceExporter`        | no       | OTLP              | Override the trace exporter.                          |
| `MetricExporter`       | no       | OTLP              | Override the metric exporter.                         |
| `OnError`              | no       | nil               | Receives async SDK errors (exporter failures, dropped batches) and multi-handler write errors. |

If `OTLPEndpoint` has no port, the default port for the chosen transport
is filled in automatically. Setting a per-signal exporter override
enables that signal even when `OTLPEndpoint` is empty.

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

The bundle's providers and propagator are reachable via `tel.OTel()`
for wiring third-party instrumentation libraries that ask for them:

```go
import "go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

h := tel.OTel()
client := &http.Client{
    Transport: otelhttp.NewTransport(http.DefaultTransport,
        otelhttp.WithTracerProvider(h.TracerProvider),
        otelhttp.WithPropagators(h.Propagator)),
}
```

Most callers never need `tel.OTel()` — the day-to-day handles
(`Logger`, `Tracer`, `Meter`) cover normal use.

The one exception to "no globals": if `Options.OnError` is set, `Init`
installs `otel.SetErrorHandler` so async SDK errors reach you. Opt-in
only — leaving `OnError` nil keeps all globals untouched.

### Last-resort global registration

If you discover a dependency that reads `otel.GetTracerProvider()`
unconditionally and silently drops spans, opt in at your `main`:

```go
h := tel.OTel()
otel.SetTracerProvider(h.TracerProvider)
otel.SetTextMapPropagator(h.Propagator)
```

Never call these inside library code — they belong in `main`.

### Reaching the underlying handles

- `tel.Logger.Slog()` / `spanLog.Slog()` return the underlying
  `*slog.Logger` for libraries that take a stdlib logger directly.
- `tel.Tracer.OTel()` returns the underlying `trace.Tracer`.
- `spanLog.Span()` returns the bound `trace.Span` — used to end the span,
  record errors, set attributes, etc.

## Behavioural contract

- `OTLPEndpoint == ""` and no exporter overrides: logs print to stdout;
  traces/metrics use noop providers. No exporters are created, no
  goroutines started.
- `OTLPEndpoint != ""` (or any per-signal exporter override): logs fan
  out to stdout **and** the configured exporter; a downstream failure on
  one record does not suppress stdout for that record.
- `tel.Flush(ctx)` runs `ForceFlush` on all enabled providers under the
  caller's `ctx`. Safe to call any number of times.
- `tel.Shutdown(ctx)` flushes and tears down providers under the
  caller's `ctx` — pick a deadline that matches your environment.
  Calling it twice is a no-op.

## Development & testing

Everything runs in Docker — no local Go toolchain needed.

```sh
./run-tests.sh
```

This builds a `golang:1.25-alpine` image, mounts the working tree into
it, and runs `go vet ./...` followed by `go test -race ./...`. The module
cache and build cache live in a named volume (`go-cache`), so subsequent
runs are fast.

The same script runs in CI on pull requests and pushes to `main`
(`.github/workflows/test.yml`).
