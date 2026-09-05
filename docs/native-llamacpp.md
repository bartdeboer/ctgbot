# Native llama.cpp backend (first version)

Run inference on the macOS host using Metal, while leaving agent tools in their
existing Docker sandbox. Linux native processes are also supported. Docker remains
the default; this does not implement native agent sandboxes or change toolloop.

## Configure

In the existing llamacpp component profile's `runtime.json`:

```json
{
  "driver": "native",
  "native": {
    "executable": "/Users/bart/.llama-app/llama",
    "args": ["serve"]
  }
}
```

Use the actual absolute executable path on the target. No shell expansion of `~`
and no login-shell PATH is assumed. For a standalone `llama-server` executable,
use its absolute path and omit `args`. Arguments are passed as argv, not shell text.
The b10679 unified llama app supports the `serve` prefix; actual Mac binary/signal
behavior still needs a target smoke test. Native execution is explicit, never an
automatic fallback from Docker errors. Restart ctgbot after changing configuration.

Keep model downloads in the model manager. Use existing `model help`,
`model install <name> <url>` or `model register <name> <path>` routes; consult their
help for context/port settings. For a large download, use the host CLI under
operator supervision rather than assuming a Hostbridge request will survive it.

Initial candidate: ggml-org/Qwen3.8-27B-GGUF,
Qwen3.8-27B-Q4_K_M.gguf (~19GB), named `qwen3.8-27b-q4`.
Pin and verify artifact revision/SHA256 when installing; no checksum is asserted
by this runtime change. Start with 16384 context and one concurrent request.
Do not enable vision, MTP, auto-download or router mode in the initial test.
ctgbot always passes the registered local model with `-m`.

Native inference and start/stop/status are **resident-run-only** in this first
version. Start `ctgbot run` and invoke existing llamacpp commands through that
instance's chat or Hostbridge. Standalone CLI lifecycle/inference commands are
rejected with guidance; they cannot control the resident process. Configuration,
help, model download and registration via the host CLI remain available because
they do not spawn inference. Bind llamacppagent to the registered model. Existing
inference session and idle behavior is reused.

## Direct-process ownership

One native service handle is retained per component/model identity. Rebinding the
same specification reuses that owner. Changing a bound specification requires a
ctgbot restart rather than silently retargeting a running service.

ctgbot owns only the **direct foreground process it starts**. It sends SIGTERM,
allows bounded grace, then kills that process if necessary and waits for its exit.
Successful Stop certifies direct-child completion only, not descendant termination.
Signal errors, wait failures and cleanup deadlines are returned as errors.

Prefer direct `llama serve -m <local-file>` or `llama-server -m <local-file>`.
A wrapper must either **exec** the server or handle termination, forward it to its
children and wait for them. ctgbot cannot enforce this. A noncooperating wrapper
can leave inference running and GPU memory allocated after the wrapper stops.
Daemonizing wrappers and model-router setups are outside this contract.

The shared process is not tied to the first request's lifetime. Failed startup is
cleaned up, and orderly `ctgbot run` exit stops owned native processes. Docker
keep-running behavior is unchanged.

Output is retained as a bounded 64KiB in-memory tail, included in startup errors.
There is no persistent log or dedicated log command. After an unclean ctgbot exit,
an orphan may remain: inspect it manually. Startup refuses an occupied port;
there is no automatic orphan recovery or adoption of existing processes.

## Networking and acceptance gates

Native servers listen on 127.0.0.1. `expose_to_sandboxes` does not change this to an
unauthenticated LAN listener. Test Docker Desktop access via host.docker.internal
with a harmless loopback service first. If inaccessible, stop and resolve that
network boundary separately; do not silently enable 0.0.0.0.

Target validation remains pending in two separate stages:

1. Actual macOS helper-process tests: graceful termination, forced kill, startup
   cancellation, spontaneous exit/restart and resident shutdown.
2. Actual llama/Metal acceptance: GPU offload, completion and tool calls, sandbox
   connectivity, memory pressure and release after shutdown.

Linux tests and a successful macOS cross-build do not certify either stage.

This change does not fix pre-existing inference session/manual-stop/idle races,
add authentication, or overhaul model downloads. Readiness checks reject occupied
ports and watch the owned process; HTTP health is not cryptographic server identity
and the port check cannot eliminate every bind race.
