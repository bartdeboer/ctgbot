# ctgbot Agent Guide

## Purpose

`ctgbot` is an agentic engineering platform with chat-bound components,
containerized agent runtimes, and a narrow `hostbridge` back to the host.

The project’s main responsibilities are:

- run chat-bound component graphs through `ctgbot run`
- provide a host-command bridge via `hostbridge` and `ctgbot hostbridge serve`
- support operator setup and runtime CLI surfaces:
  - `ctgbot component <component> auth`
  - `ctgbot component <component> auth status`
  - `ctgbot image build`
  - `ctgbot chat <chat> component add ...`
  - `ctgbot chat <chat> component <source> filter add ...`

## Repo Shape

- `cmd/ctgbot`: main CLI entrypoint
- `cmd/hostbridge`: container-side hostbridge client
- `cmd/pack`: generates the embedded Docker build context tarball
- `internal/appstate`: typed config access and local/global state helpers
- `internal/message`: shared inbound/outbound transport types
- `internal/commandengine`: typed command registry, router, authorization, and execution
- `internal/configengine`: config registry/get/set enforcement
- `internal/schema`: shared command and config definitions
- `internal/hostbridge`: hostbridge request/response transport, client, and server
- `internal/buildassets`: embedded Docker build context tarball source
- `internal/runtime/image`: shared runtime image build helpers
- `internal/broker`, `internal/component`, `internal/system`, `internal/runtime`: the real runtime path
- `docker/codex.Dockerfile`: Codex runtime image
- `docker/claude.Dockerfile`: Claude runtime image

## Main Commands

- `go run ./cmd/ctgbot run`
  Runs the live runtime.

- `go run ./cmd/ctgbot workspace list`
  Lists configured workspaces.

- `go run ./cmd/ctgbot component list`
  Lists registered components.

- `go run ./cmd/ctgbot component codex/codex auth`
  Runs the component-scoped auth flow for the default Codex registration.

- `go run ./cmd/ctgbot component codex/codex auth status`
  Shows authentication status for the default Codex registration.

- `go run ./cmd/ctgbot hostbridge serve`
  Starts the hostbridge server over TCP.

- `go run ./cmd/ctgbot process install`
- `go run ./cmd/ctgbot process upgrade`
- `go run ./cmd/ctgbot process quit`
- `go run ./cmd/ctgbot image build --no-cache`
- `go run ./cmd/ctgbot go-generate`

The top-level `install`, `upgrade`, and `quit` commands are CLI aliases for:

- `process install`
- `process upgrade`
- `process quit`

## Runtime Layout

There are three important state locations:

- `./.ctgbot`
  Local runtime state for the current project directory.
  Contains:
  - `config.json`
  - `ctgbot.db`
  - component homes under `components/<type>/<name>`
  - chat-local fallback workspaces under `chats/<chatID>/workspace`
  - hostbridge TLS state

- configured named workspaces
  Loaded from root config and pointed at real host folders.

`./.ctgbot` is runtime data and should not be committed.

## Runtime Model

The important runtime concepts are:

- `Workspace`
- `Component`
- `Runtime`
- `Chat`
- `ChatComponent`
- `ThreadComponentMapping`
- `ThreadComponentState`
- `Broker`

Static runtime settings come from component-home `runtime.json`.
Static component settings come from component-home `component.json`.
Mutable per-thread component settings belong in `ThreadComponentState`.
Component-scoped authentication flows resolve through registered component refs.
The canonical CLI form is `component <component> ...`.

For the current public overview and setup flow, see `README.md`.

## Known Operational Notes

- Codex component auth uses the callback relay opened by the runtime.
  The default callback port is `127.0.0.1:1455`.

- After changing Dockerfiles or anything under the embedded build context, regenerate and rebuild:
  1. `go run ./cmd/ctgbot go-generate`
  2. `go run ./cmd/ctgbot image build --no-cache`

## Editing Guidance

- Keep `cmd/` files focused on `clir` routing and small orchestration.
- Keep runtime image helper logic in honest packages like `internal/runtime/image`.
- Keep runtime behavior in `internal/broker`, `internal/component`, `internal/system`, and `internal/runtime`.
- Keep message transport types in `internal/message`.
- Keep config/state access in `internal/appstate`.
- Prefer updating the embedded build-context source files and then regenerating `internal/buildassets/assets/src.tar.gz`.
- Do not commit local runtime data from `./.ctgbot/`.

## Good First Checks

If something breaks, check these first:

1. `go run ./cmd/ctgbot config`
2. `go run ./cmd/ctgbot component list`
3. `go run ./cmd/ctgbot component codex/codex auth status`
4. `go run ./cmd/ctgbot image build --no-cache`
5. whether the Docker image is stale after changing Dockerfile or embedded assets
