---
name: telemetry-events
description: >-
  Add telemetry events to the RWX CLI. Covers event naming, prop conventions,
  how to thread the collector into internal packages, how sentinel errors map
  to `cli.error` `error_type` buckets (and the trap of returning unwrapped
  errors that classify as `unknown`), and how to test event recording. TRIGGER
  when: the user asks to "add telemetry", "log a telemetry event", "instrument",
  "track usage of", "record metrics for"; OR when adding a new error path /
  sentinel error and asking how it interacts with telemetry / `cli.error` /
  `classifyError`; OR when an error is showing up as `error_type: unknown` in
  telemetry.
---

# Adding telemetry events

The CLI has two telemetry types in `internal/telemetry/`:

- **`*telemetry.Collector`** — thread-safe event queue. Method: `Record(event string, props map[string]any)`. This is what you pass into internal packages.
- **`*telemetry.Telemetry`** — orchestrator that holds a Collector + Sender + StatsRoundTripper, and is responsible for `Flush()` at process exit. Used at the cmd layer.

Two package-level vars are set up in `cmd/rwx/root.go`:

```go
var (
    telem              *telemetry.Telemetry  // top-level, for cmd/rwx/*.go
    telemetryCollector *telemetry.Collector  // pass into internal packages
)
```

## Event naming

Use dotted `<area>.<action>` names. Existing events:

- `cli.command` — every top-level command invocation (in `cmd/rwx/main.go`)
- `cli.error` — every command-level error (in `cmd/rwx/main.go`)
- `api.summary` — aggregated API call stats (in `internal/telemetry/stats_roundtripper.go`)
- `lsp.node_check` — Node version check inside `findNode()` (in `internal/lsp/serve.go`)

Pick a name that names the **subsystem** and the **observable moment**, not the
outcome. Use a `status` prop for outcome buckets — see below.

## Recording from `cmd/rwx/`

The cmd layer can use the `telem` global directly:

```go
telem.Record("cli.command", map[string]any{
    "command":     commandName,
    "duration_ms": time.Since(start).Milliseconds(),
    "success":     err == nil,
})
```

`*telemetry.Telemetry` is nil-tolerant — `telem.Record()` is a no-op if the
pipeline failed to initialize. No nil checks needed.

## Recording from internal packages

Internal packages should take `*telemetry.Collector` as an **explicit
dependency** rather than reaching for a global. Two patterns in use:

**Pattern A — struct field** (when the package already has a long-lived
service-style struct, e.g. `internal/cli/service.go`):

```go
type Service struct {
    // ...
    TelemetryCollector *telemetry.Collector
}

func (s Service) recordTelemetry(event string, props map[string]any) {
    if s.TelemetryCollector == nil {
        return
    }
    s.TelemetryCollector.Record(event, props)
}
```

**Pattern B — function parameter or config-struct field** (when the package
exposes free functions, e.g. `internal/lsp/`):

```go
// Function parameter
func Serve(collector *telemetry.Collector) (int, error) { ... }

// Or via a config struct
type CheckConfig struct {
    // ...
    TelemetryCollector *telemetry.Collector
}
```

Inside the function, nil-tolerate with a small closure:

```go
record := func(props map[string]any) {
    if collector == nil {
        return
    }
    collector.Record("lsp.node_check", props)
}
```

Then wire it from `cmd/rwx/`:

```go
// cmd/rwx/lsp.go
exitCode, err := lsp.Serve(telemetryCollector)

// cmd/rwx/lint.go
cfg.TelemetryCollector = telemetryCollector
```

**Always nil-tolerate.** Telemetry is best-effort and must never block the CLI.
Tests, MCP entry points, and library consumers may all pass nil.

## Prop conventions

Props are `map[string]any`. Conventions in the existing codebase:

- **Bucket categorical state in a `status` string**, not booleans. E.g.
  `lsp.node_check` uses `"status": "missing" | "version_check_failed" |
  "unparsable" | "too_old" | "eol_warning" | "ok"`. This makes future buckets
  additive without breaking dashboards.
- **Include the raw value alongside the bucket** when it's useful for
  drill-down (e.g. `version: "v18.0.0"` next to `status: "eol_warning"`).
- **Use snake_case keys** (`duration_ms`, `error_type`, `output_format`).
- **Strip PII / large strings** before recording. See
  `cmd/rwx/main.go:scrubErrorMessage` for the existing approach to error
  messages.
- The envelope (timestamp, OS, arch) is added automatically — don't include
  those in props.

## Errors and the `cli.error` event

Every command-level error is recorded as a `cli.error` event in
`cmd/rwx/main.go:recordTelemetry`, with two derived props that come from
`classifyError(err)` and `errors.Is(err, HandledError)`:

- `error_type` — a short bucket like `bad_request`, `unauthenticated`,
  `not_found`, `lsp_error`, `timeout`, `sandbox_setup_failure`, etc., derived
  by `errors.Is(err, internalerrors.ErrXxx)` against the sentinel list in
  `internal/errors/errors.go`. Falls through to `"unknown"` if nothing matches.
- `handled` — true if the error wraps `HandledError`, meaning we already
  printed a friendly message to the user.

For unclassified, non-handled errors, `error_message` is also attached, run
through `scrubErrorMessage` (strips home dir, URL credentials, JWTs, and
token-shaped runs; truncates to 200 runes).

### The sentinel-wrapping rule

**If you add a code path that returns a new error and don't wrap it with a
sentinel, every failure on that path will telemetry-classify as `unknown` and
disappear from your alerting buckets.** This was the bug behind commit
`bdae1f1`: lint failures were all `unknown` until `ErrLSP` and `ErrTimeout`
sentinels were added and `WrapSentinel` was applied at the LSP boundary.

When adding a new error path:

1. **Check the existing sentinels** in `internal/errors/errors.go`. If one
   fits semantically (`ErrTimeout`, `ErrLSP`, `ErrSSH`, `ErrPatch`,
   `ErrUnauthenticated`, `ErrNetworkTransient`, etc.), wrap with
   `errors.WrapSentinel(err, errors.ErrXxx)` at the point where the error
   becomes recognizable as that category. `errors.Is` will then match it.
2. **If it's a genuinely new category**, add a sentinel to
   `internal/errors/errors.go` and a `case` to `classifyError` in
   `cmd/rwx/main.go`. Update `cmd/rwx/main_test.go:TestClassifyError`. Pick a
   short snake_case bucket name — these become alert filters and dashboard
   rows, so keep them stable and meaningful.
3. **Wrap at the right layer.** The deepest-knowing layer (where the category
   is unambiguous) is usually the right place. E.g. JSON-RPC timeouts get
   wrapped in `internal/lsp/jsonrpc.go`; LSP protocol failures get wrapped at
   each LSP request site in `internal/lsp/check.go`.

`WrapSentinel` preserves the wrapped error's message and chain, captures a
stack trace, and makes `errors.Is(err, sentinel)` return true. Don't use plain
`fmt.Errorf("...: %w", err)` for sentinels — `WrapSentinel` is the convention
because it also captures the stack for `%+v` rendering.

### `HandledError`

`HandledError` (in `cmd/rwx/main.go`) marks errors that have already been
reported to the user — wrapping with it tells the cmd layer to suppress the
generic error printing and sets `handled: true` on `cli.error`. Use it for
errors where you've already produced your own user-facing output (e.g.
`LintFailure = errors.Wrap(HandledError, "lint failure")`).

## Testing

Use a real `*telemetry.Collector` and `Drain()` to inspect events. No mocking
needed:

```go
collector := telemetry.NewCollector()
_, _, err := findNode(collector)
require.NoError(t, err)

events := collector.Drain()
require.Len(t, events, 1)
require.Equal(t, "lsp.node_check", events[0].Event)
require.Equal(t, "ok", events[0].Props["status"])
require.Equal(t, "v22.4.1", events[0].Props["version"])
require.Equal(t, 22, events[0].Props["major"])
```

Existing tests already passing nil for the collector should continue to do so;
only add a real collector in tests that specifically assert telemetry behavior.

## Checklist for a new event

- [ ] Event name follows `<area>.<action>` and matches the subsystem.
- [ ] Recording site fires on **every** code path that reaches it (success and
      every error path) — don't drop events on the failure branches.
- [ ] Props bucket categorical state in a `status` field.
- [ ] Collector is threaded as an explicit dependency, not a global.
- [ ] Recording site is nil-tolerant.
- [ ] Test asserts the event fires with expected props for at least the happy
      path and the most interesting failure bucket.
- [ ] No PII or unbounded strings in props.

## Checklist for a new error path

- [ ] Errors returned to the cmd layer wrap a sentinel via
      `errors.WrapSentinel`, so `cli.error` doesn't classify them as `unknown`.
- [ ] If no existing sentinel fits, a new one was added to
      `internal/errors/errors.go` and a `case` to `classifyError` in
      `cmd/rwx/main.go` (with a corresponding `TestClassifyError` case).
- [ ] User-facing errors that are already printed wrap `HandledError`.
