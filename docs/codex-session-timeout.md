# Codex turn timeout

Codex turns have **no added wall-clock deadline by default**. This permits
hours- or days-long attached execution; it is not a detached job/recovery
guarantee. Parent context cancellation and operator interrupt still apply.

The existing root setting `codex.session-timeout` controls an optional deadline:

- Unset, empty, or zero (`0`, `0s`): no added deadline.
- Positive durations such as `30m` or `48h`: explicit per-turn deadline.
- Bare integers still mean minutes, for compatibility.
- Negative, malformed, and overflowing values are errors, not unlimited execution
  or a silent fallback. Use hours (e.g. `48h`), not `2d`.
- The persisted key remains `session.timeout_min`. Existing positive settings
  remain effective: this change does not erase an explicitly stored ten minutes.
- An invalid persisted value is reported by config reads and prevents the runner
  from starting execution. Invalid writes are rejected before persistence.

Previously unset, zero, and malformed values fell back to ten minutes.
Negative values bypassed the timeout, and bare integer multiplication could
overflow. Those accidental behaviors are deliberately not retained.
No other provider's timeout policy changes.

## Cancellation / ownership limitation (not fixed here)

The current code path is:

1. `broker.HandleResolvedInbound` runs the turn through `ThreadTurnGate.Run`
   (`internal/broker/broker.go`).
2. The gate defers release until its callback returns
   (`internal/broker/thread_turn_gate.go`).
3. Codex `Runner.RunTurn` passes its context to runtime exec.
4. Docker execution uses `exec.CommandContext(ctx, "docker", ...)`, followed
   by `cmd.Run()` (`internal/containerengine/spec.go`).
5. When RunTurn returns, Codex skips StopAfterTurn if `keep_running` is true
   (`internal/component/codex/component.go`).

Canceling the Docker client does not establish that the container-side Codex
process has exited. The callback can therefore return and release the in-process
turn gate while the original process continues legitimate work. A subsequent
resume can encounter the provider's active-writer lock.

Removing the implicit deadline prevents that particular automatic cancellation
trigger. It does **not** fix explicit timeouts/cancellation, remote process
ownership, overlapping resume prevention, or restart recovery. This patch adds
no automatic kill, retry, reset, session deletion, or lifecycle framework.

The incident's effective remote configuration/deadline has not been verified;
the above is code-path evidence, not attribution of that incident to the default.
