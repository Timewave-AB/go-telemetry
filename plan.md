# go-telemetry — improvement plan

Status: proposed, 2026-08-30. Basis: an architecture review and a product-owner
review of `main` at `a7272f1`, plus three findings verified directly against the
tree (§3).

The reviews agreed, from different angles, on one diagnosis: **the repository
states no goals at all**, and the goal it is most obviously missing is adoption.
The code itself is careful and well tested — 60 tests, `vet`/`-race` clean — so
nothing here blocks on code quality. It blocks on contract and purpose.

---

## 1. Decisions

Taken 2026-08-30. Everything below is ranked against these; if one changes, the
ranking changes with it.

| # | Decision | Note |
|---|---|---|
| D1 | **Genuinely public library**, not an internal-only house package | Outside users are a target audience. Both reviews recommended the opposite ("house package, MIT for convenience"); this overrides them, and §2/§4 are written to the decision, not to the recommendation. |
| D2 | **Mandated for Timewave Go services** | The mandate is an internal fact and belongs in the labs' decision records — never in the public README. |
| D3 | **Minimum Go = the lowest version that compiles** | 1.25 today. Raised only when a dependency forces it, never to chase a release. |

### What D1 changes relative to the reviews

| Reviews assumed (house package) | Under D1 (public library) |
|---|---|
| `OTEL_*` env vars → explicit **non-goal** | **Goal** (§2.2 item 5). Zero `OTEL_*` support was called "disqualifying for outside users". |
| Fixed stdout format is a virtue; configurability is a tax | Fixed *default* with a seam. An outsider with a JSON pipeline must not be locked out. |
| Cross-language parity with `timewave/logger` (PHP) and `@larvit/log` (Node) as a candidate goal | Drops out of the *public* goals; remains an internal convention. |
| Flattening `internal/core` = tidiness, "half a day, mechanical" | Near-top priority. pkg.go.dev is the storefront. |
| Put "Current consumers: 0" in the README | Drop it. That is a house-package honesty device; on a public library it repels users. |

---

## 2. Goals

To be adopted into `README.md`, replacing nothing (there is currently no such
section).

### 2.1 Who this is for

> Go services that ship logs, traces and metrics to an OTLP collector and want a
> house log format on stdout at the same time. Developed and used in production
> at Timewave, published as a supported public library — outside users are a
> target audience, not an accident.
>
> If you have no opinion about your stdout format, the OTel SDK plus `otelslog`
> will fit you better. This package earns its place when you want one fixed,
> human-readable line in `docker logs` *and* structured OTLP export, from one call.

### 2.2 Goals

1. **One call to production telemetry.** A service reaches "logs in Loki, traces
   in Tempo, metrics in Prometheus" with one `Init` and no OTel SDK knowledge.
2. **Telemetry never takes the service down.** An absent, unreachable or
   misconfigured collector degrades to stdout logging. `Init` fails only on
   programmer error.
3. **Correlation you can actually follow.** A span-bound logger tags its records
   with `trace_id`/`span_id` on every leg the operator reads — stdout included —
   and an inbound W3C `traceparent` puts the service under its caller's trace.
4. **A fixed, readable line by default — and a seam if you disagree.** The
   default stdout format is stable and identical across services. It is a
   default, not a cage: the handler is exported and the writer injectable.
5. **Predictable in the Go ecosystem.** Standard `OTEL_*` environment variables
   are honoured, `Options` overrides them, and nothing is registered on the OTel
   globals unless you ask.
6. **The escape hatch is always open.** Providers, propagator, tracer and the
   underlying `*slog.Logger` are reachable, so no consumer is ever stuck behind
   this package.
7. **Call sites depend on our types, not OpenTelemetry's.** The day-to-day
   handles are house types, so replacing what sits underneath them is a change
   to this package rather than an edit to every call site in every service.
   This is a migration-cost goal, not a binary-size one — see §3 V4.

### 2.3 Non-goals

- **Backends other than OTLP.** Exporter overrides are a seam for tests and
  unusual cases, not a plugin system.
- **Metrics conventions.** Metric names, units and label cardinality are the
  caller's; the package supplies instruments, not a taxonomy.
- **A full facade over the OTel metrics API.** The house instruments (W12) are
  a deliberate subset. Anything they do not cover goes through
  `tel.OTel().MeterProvider`.
- **Auto-instrumentation.** `otelhttp`, `otelgrpc` and friends wire through
  `tel.OTel()`.
- **Log sampling or rate limiting.** Use `Level`, or the collector.

### 2.4 Support contract

- **Minimum Go: 1.25** — the lowest version the dependency graph compiles on.
- **`v0.x`: the API may break on any minor.** Pin an exact version.
- Issues and PRs welcome. No SLA.

---

## 3. Verified findings

Checked directly against `a7272f1`, not taken from the reviews on trust.

### V1 — The Go 1.26 floor is self-imposed

`go.mod:3` declares `go 1.26.0`. Every module in the dependency graph tops out at
`go 1.25.0` (otel v1.46.0, otelslog v0.20.1, grpc v1.83.1, `golang.org/x/*`);
go-telemetry's own directive was the only thing above it. With `go 1.25.0`
substituted, the full suite passes under go1.25.14:

```
go vet ./...      # clean
ok  github.com/Timewave-AB/go-telemetry               1.017s
ok  github.com/Timewave-AB/go-telemetry/internal/core 11.179s
```

This single line is what forces a consumer on Go 1.25 to bump its toolchain
before it can adopt at all.

### V2 — `Init` can refuse to start the service

`internal/core/resource.go:17` calls `resource.WithProcess()`, which resolves
`process.owner` through `user.Current()`. In a `CGO_ENABLED=0` binary running
under a UID absent from `/etc/passwd` — `docker run --user`, a compose
`user: "${UID}:${GID}"` line, k8s `runAsUser`, distroless, scratch — that fails.
`internal/core/init.go:82-85` treats any resource error as fatal, and the
README's own example does `panic(err)`.

`resource.New` returns a usable partial resource alongside that error; it is
discarded. A cosmetic attribute therefore stops the process booting.

### V3 — stdout carries no trace id

`README.md:46-49` promises that `log.Info(...)` through a `*SpanLogger` is
"tagged with the span's `trace_id` and `span_id`". On stdout it is not:
`internal/core/handler_text.go:38` is
`func (h *textHandler) Handle(_ context.Context, r slog.Record) error` — the
context is discarded — and nothing outside `_test.go` writes a trace id into a
text line.

The godoc is accurate; `internal/core/tracer.go:52` says correlation happens "via
the otelslog bridge", i.e. the OTLP leg only. The README drops that qualifier.

Consequences: an operator reading `docker logs` cannot pivot to a trace, and with
`OTLPEndpoint: ""` — the configuration the README presents as the simple case —
the headline feature does nothing at all while the tracer is a noop, so someone
evaluating the library sees the promise fail and cannot tell it is by design.

### V4 — The dependency graph is large, and no facade shrinks it

Measured 2026-08-30 against `a7272f1`, `CGO_ENABLED=0`, `-ldflags="-s -w"`:

| Build | Packages | Stripped binary |
|---|---|---|
| `Init` with all three signals | 458 | 15.7 MB |
| logs + traces only | 407 | 14.4 MB |
| logs + traces + metrics | 427 | 15.3 MB |

The metrics path costs **+20 packages and ~948 KB (~6%)**. The weight is the
transport: 109 of the 458 packages are gRPC/protobuf, and 14.4 MB is already
spent before metrics enter.

Two conclusions, both load-bearing for W12:

1. **A house wrapper removes zero bytes.** Go's build graph follows what the
   implementation imports, not what the API exposes. Wrapping `Meter` does not
   stop this package importing `sdkmetric`; it stays in `go.mod` and in every
   consumer's binary. Only compile-time separation — a sub-package or module
   split, or build tags — removes a package from a consumer's build, and
   *runtime* per-signal enablement (§5 item 7) saves nothing at all.
2. **If binary size is ever the goal, metrics is the wrong target.** Defaulting
   to `otlphttp` instead of gRPC would save several times what removing metrics
   could.

So W12's justification is migration cost — the OTel API surface is large and its
transitive graph is messy, so pinning consumer call sites to house types keeps a
future swap inside this package. It is explicitly *not* a size optimisation, and
should never be recorded as one.

---

## 4. Workstreams

Ordered. Sizes are rough: XS ≈ under an hour, S ≈ half a day, M ≈ one to two days.

### W1 — Drop the Go floor to 1.25 — XS

`go.mod:3` → `go 1.25.0`; `Dockerfile` → `golang:1.25-alpine` (full patch pin per
convention). Verified green (V1).

*Acceptance:* `./run-tests.sh` green; a consumer on Go 1.25 with
`GOTOOLCHAIN=local` can `go get` the module.

### W2 — Make `Init` fail-open — XS

Drop `resource.WithProcess()` (keep `WithProcessPID` if the pid is wanted), or
accept the partial resource on `ErrPartialResource`. Report through `OnError`
rather than returning fatally. Keep a strict variant if one is wanted, but the
default must not stop a service booting.

*Acceptance:* a regression test running with a resource detector that errors still
returns a usable `*Telemetry` logging to stdout. Goal 2.

### W3 — Flatten `internal/core` into root `package telemetry` — S

Move `internal/core/*.go` to the root as `package telemetry`; delete
`telemetry.go` (the alias re-export file); keep `api_test.go` as
`package telemetry_test` for black-box coverage and the rest as in-package tests.

The type aliases give zero encapsulation — `core.Options` *is* `telemetry.Options`,
every exported field is public, and changing core breaks the public API
identically — while costing the entire public API reference: `go doc` on the
module renders only a list of aliases. `Options`' twelve documented fields and
every `Logger`/`Tracer`/`SpanLogger` method render nowhere, and pkg.go.dev builds
from the same source. It also removes a naming defect: `core` names nothing.

At ~2,700 lines the root would hold ~11 source files, well inside a readable flat
tree. If it later grows, group by concern (`handler/`, `otlp/`).

*Acceptance:* `go doc github.com/Timewave-AB/go-telemetry` renders `Init`,
`Options` with its field docs, and all logger/tracer methods.

### W4 — Put `trace_id`/`span_id` on the stdout line — S

Stop discarding the context in `textHandler.Handle`; when the record's context
carries a valid `SpanContext`, emit `trace_id=` and `span_id=`. Fixes V3, makes
Goal 3 true on the leg the operator actually reads, and is the single change that
makes the package visibly better than the copy it competes with.

*Acceptance:* a span-bound log line on stdout carries both ids; a context-free
line carries neither; format tests cover both.

### W5 — Honour `OTEL_*` environment variables — M

New under D1; both reviews recommended making this an explicit non-goal, and D1
reverses that. At minimum `OTEL_EXPORTER_OTLP_ENDPOINT`, `OTEL_SERVICE_NAME`,
`OTEL_RESOURCE_ATTRIBUTES`. Precedence: env is read first, `Options` overrides it.
Document the precedence rule explicitly — a silent precedence is worse than none.

Note the current behaviour is a trap for anyone arriving with ecosystem
expectations: `OTEL_EXPORTER_OTLP_ENDPOINT` set and `Options.OTLPEndpoint` empty
means no export at all, silently.

*Acceptance:* env-only configuration produces a working exporter; `Options` set
alongside env wins; precedence is covered by tests and stated in the README.

### W6 — Open the format seam — S

Export the text handler constructor, add `Options.Output io.Writer` (defaulting to
`os.Stdout`; `internal/core/init.go:204` currently hardcodes it), and export
`ParseLevel(string) (slog.Level, error)` (`internal/core/level.go:21-39` is
unexported).

Three things drive this: Goal 4's seam for public users; the library's own tests
swap OS file descriptors and document the constraint as a workaround
(`internal/core/init_test.go:302-304`); and a consumer's degraded fallback logger
currently emits stdlib logfmt rather than the house format, so the log shape
silently changes in exactly the case that matters most.

*Acceptance:* a caller can capture log output in a `bytes.Buffer` without touching
file descriptors, and can construct the house handler directly.

### W7 — Endpoint hygiene — S

`internal/core/options.go:101` uses
`strings.LastIndex(endpoint, ":") > strings.LastIndex(endpoint, "]")` to decide
"already has a port". For `https://collector` that is `5 > -1`, so the
scheme-bearing string is passed to `WithEndpoint`, which wants a bare `host:port`.
On HTTP this surfaces as a parse error; on gRPC — the default — it fails silently
at the wire, forever.

Accept a URL: strip the scheme, infer `OTLPSecure` from it, reject anything left
that is not `host[:port]`, and fail identically on both transports. This matters
more under D1 and W5, because `OTEL_EXPORTER_OTLP_ENDPOINT` is a URL by
specification.

*Acceptance:* `https://collector`, `http://collector:4318` and `collector:4317`
all behave predictably; a malformed endpoint errors at `Init` on both transports.

### W8 — Amend the labs' decision records — XS plus a conversation

`core-4-go-lab/docs/lab/decisions.md:80` records
`Logging | copy queue-worker/logger | ~20 modules | org standard` and does not
mention this library — not as chosen, not as rejected. The auth-4 record is
equivalent. Under D2 both must name go-telemetry and record the decision.

This is the highest-leverage item in the plan and no work inside this repository
substitutes for it. There are already two copies of one 257-line file, and the
`auth-4-go-lab` copy is the *better* one — it independently fixed a level-filtering
bug and a dropped-groups bug that are still live in `queue-worker/logger`, and
neither fix flowed back. That is copy-paste drift in progress with the fixes
travelling the wrong way. The window is that core-4-go-lab has no Go code yet, so
the third copy has not happened.

The argument that wins this is **not** the log format. It is `Tracer.Extract`
(`internal/core/tracer.go:43`): the only Go implementation in the org that can put
a service under its caller's trace in Tempo, which is what the traceparent work
needs. `auth-4-go-lab/logger/trace.go` hand-parses `traceparent` and hangs string
attributes on a `*slog.Logger` with no tracer provider at all — nothing reaches
Tempo. That argument currently sits at README line 88 and should lead.

### W9 — Make it runnable: examples and an end-to-end test — M

`compose.yml` contains one service (the test runner) and no collector, so there is
no way to confirm the library works short of deploying it. Add an `_examples/`
module with a working `main.go` and a compose file running `otelcol`, plus godoc
`Example` functions — there are currently none, so pkg.go.dev's Examples tab is
empty, and examples would also lock the README against compile drift.

Add an end-to-end test asserting on what reaches the wire: level gating on both
legs, resource attributes, and one span with a joined `traceparent`. Both
TW-13164 defects were invisible to a suite with 100%/85.6% coverage because
coverage was never the gap — configuration was, and the untested configuration is
the one production runs.

*Acceptance:* `docker compose up` in `_examples/` produces a trace and correlated
logs against a local collector; the e2e test fails if the OTLP level gate regresses.

### W10 — Land and keep the first consumer — M, in flight

queue-worker (TW-12457) is mid-adoption and is the natural first consumer, being
the origin of the extracted code. W1 removes its toolchain bump; W2 removes the
`InitOrFallback` wrapper it had to write; W6 removes roughly 40 further lines of
its adapter. Dogfooding is how a public library earns credibility, and this
adoption attempt is what surfaced both TW-13164 defects in the first place.

*Acceptance:* queue-worker on `main` depends on a tagged release, with no local
logger and no adapter working around a library gap.

### W11 — Smaller corrections — S in total

- `internal/core/otlp_traces.go:47-54`: `ratio <= 0` and `ratio >= 1` return the
  same sampler; collapse the dead branch. A negative or `>1` ratio is accepted
  silently, and there is no way to express "never sample" — `-1` silently means
  "always". Validate.
- `internal/core/resource.go:15` passes `context.Background()` while `Init` holds
  a live `ctx` (`internal/core/init.go:82`); a cancelled `Init` still runs detection.
- Split the level processor out of `internal/core/otlp_logs.go` into
  `level_processor.go`. It closes a real SDK gap and is the part a reader needs to
  find; the file currently also holds provider and exporter construction.
- Wrap exporter errors with the signal name and the resolved endpoint. A dead
  collector currently yields three bare `context deadline exceeded` lines joined
  together.
- Level naming drifts across the docs: `README.md:138` says `warn`,
  `README.md:175` says `warning`, the constant is `LevelWarning`, output renders
  `WARN`. Pick one spelling for the docs.
- `README.md:31` still uses `ServiceName: "queue-worker"` from a service that had
  not adopted the library. Cosmetic, but it is the first thing a reader sees.
- `Options.ServiceVersion` has three contradictory contracts: the README table
  says optional, `internal/core/options.go:35` says "required unless
  ReadBuildInfo can supply one", and `internal/core/init.go:73-75`'s "required"
  error is unreachable — `resolveServiceVersion` always returns non-empty,
  falling back to `"unknown"`. For a `go build` binary `Main.Version` is
  `"(devel)"`, filtered at `internal/core/init.go:254`, so the real-world result is
  `service.version="unknown"` on every Docker-built binary. Document it as
  effectively required and pass a real value in the example.
- Document the failure model: what a dead collector drops, why
  `defer tel.Shutdown(ctx)` must not swallow its error, what a forgotten
  `log.Span().End()` costs. `Init` never dials, so a typo in an endpoint produces
  a service that looks healthy and is invisible in Grafana; the documented usage
  guarantees nobody sees it.
- Write down *why* the three exporter builders stay separate
  (`otlp_logs.go:65-83`, `otlp_traces.go:26-45`, `otlp_metrics.go:29-47` share one
  shape but the SDK's option types are unrelated) and *why* `Meter` is unwrapped.
  Silence makes deliberate duplication indistinguishable from drift.

### W12 — House metric instruments, as a narrow subset — S

Wrap `Meter` so consumer call sites reference house types. Deliberately a
*subset*, not a facade — that is what keeps it small enough to be worth having:

```go
type Attr struct {
	Key   string
	Value any
}

type Counter interface {
	Add(ctx context.Context, n int64, attrs ...Attr)
}

type Histogram interface {
	Record(ctx context.Context, v float64, attrs ...Attr)
}
```

Cover only the instrument shapes actually used; anything else goes through
`tel.OTel().MeterProvider`, which goal 6 guarantees.

The subset boundary is the whole design. A wrapper that hands back
`metric.Int64Counter` achieves nothing, because `Add` is
`Add(ctx, incr int64, options ...AddOption)` and attributes arrive via
`metric.WithAttributes(...attribute.KeyValue)` — so OTel types reach the call
site anyway. Full insulation would mean wrapping roughly eight instrument types,
the option types and the attribute type, which is re-declaring the API. Owning
`Attr` is what avoids that.

Rationale is migration cost, per §3 V4 — **not** dependency weight. Breaking
change: batch with §5 item 1 into one release.

*Acceptance:* a service can create and use counters and histograms without
importing `go.opentelemetry.io/otel/...` anywhere; an exotic instrument is still
reachable through `OTel()`; the README says which shapes are covered.

### W13 — Close the `trace.Span` leak at the call site — S

Goal 7 is not met today, and the largest breach is not the metrics API — it is
`spanLog.Span()`, which returns a `trace.Span`. The README's headline pattern is
`defer log.Span().End()`, so an OTel type appears in *every function that opens
a span*. Wrapping `Meter` while that stands would treat the smallest leak and
leave the largest.

Give `SpanLogger` the operations the call site actually needs — `End()`,
`RecordError(err)`, `SetAttributes(...Attr)` — so `Span()` becomes the escape
hatch rather than the documented path.

Decide alongside §5 item 1: both concern the logger/tracer surface, both are
breaking, and they should land in one release rather than two.

*Acceptance:* the README's span examples import no OTel package; `Span()` still
exists and is documented as the escape hatch.

---

## 5. Open questions

1. **`Logger` / `SpanLogger`.** The two types duplicate seven methods, differing
   only in which context reaches slog. The architecture review wants them
   collapsed into one type with `Ctx(ctx)`; the product review wants the split
   kept, plus `Tracer.Bind(ctx) *SpanLogger` and a shared `Log` interface both
   satisfy. Both agree the current state — two concrete types, no shared
   interface, no way to obtain a logger bound to a span someone else created — is
   wrong. The second option is less breaking and is the recommended starting
   point. Note that `SpanLogger`'s fields are unexported and `Tracer.Start` is its
   only constructor, so wiring `otelhttp` (which the README recommends) leaves you
   holding a live span you cannot log against.
2. **`Transport` spelling.** `TransportGRPC`/`TransportHTTP` versus the
   specification's `grpc` / `http/protobuf`. Under D1 outside users arrive with
   ecosystem expectations; worth renaming while still on `v0.x`.
3. **`Options.Level`.** A free-form `string` that accepts anything, while the
   exported `LevelInfo`-style constants are `slog.Level` and cannot be assigned to
   it. A typo costs one stderr line and a silent downgrade to info. `auth-4-go-lab`'s
   copy uses a typed `LogLevel string` with named constants — better ergonomics
   than the package it competes with. Related: an unknown `Transport` is a hard
   `Init` error while an unknown `Level` warns and continues; two policies in one
   struct. `auth-4-go-lab/docs/contract-gates.md:304` deliberately chose to refuse
   to boot on an unknown level, on the grounds that every line above the
   silently-chosen fallback would simply be absent. Pick one policy and state it.
4. **`deployment.environment`.** `Options` has no field for extra resource
   attributes and `buildResource` never calls `resource.WithFromEnv()`, so staging
   and production are indistinguishable in Grafana except by collector-side
   enrichment. W5 may cover this; confirm it does.
5. **Ownership of injected exporters.** `Init`'s rollback only shuts down providers
   it built, so a caller-supplied exporter is never torn down. Document who owns
   it, or take ownership.
6. **Default metric instrumentation and exemplars.** `Init` starts a full
   metrics pipeline — exporter, periodic reader, goroutine, collector connection
   — that exports nothing until the caller creates instruments; there is no
   `otelruntime` dependency and no exemplar configuration anywhere in the tree.
   Two questions follow: should `Init` ship default runtime/process metrics so
   the pipeline it starts is not empty, and should it configure an exemplar
   filter so a histogram bucket links back to a trace? Note that goal 3
   currently excludes metrics by omission — logs get trace correlation and
   traces get the traceparent join, while metrics get neither.
7. **Per-signal enablement.** `OTLPEndpoint` is all-or-nothing across the three
   signals, so "OTLP logs, no traces yet" — the position of a service mid-migration
   — is reachable only by injecting a hand-built exporter through what is
   documented as a test seam. The first adoption attempt did exactly that and
   hand-copied the exporter construction, losing the port defaulting. Consider
   explicit per-signal switches independent of the exporter overrides.

---

## 6. Suggested order

W1 and W2 together are one sitting and remove both hard stops. W8 is independent
of all code work, is the highest-leverage item, and has a closing window.

```
sitting 1   W1, W2                 both hard stops gone
sitting 2   W8                     decision records, before core-4-go-lab writes Go
then        W3, W4                 storefront and the headline feature
then        W5, W6, W7             the public-library surface
then        W9, W10                proof it works, and a real consumer
one release W12, W13, §5 item 1    the breaking API batch — house types at the
                                   call site, decided and shipped together
ongoing     W11                    fold into whichever branch touches the file
```

W12 and W13 are both breaking and both serve goal 7, so they belong in one
`v0.x` bump together with whatever §5 item 1 settles — not dribbled out across
three releases that each break consumers.
