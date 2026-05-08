# ctgbot Agent Guide

## Purpose

`ctgbot` now centers on the `v5` runtime path.

The project’s main responsibilities are:

- run chat-bound component graphs through `ctgbot v5 run`
- provide a host-command bridge via `hostbridge` and `ctgbot hostbridge serve`
- support a small operator CLI surface:
  - `ctgbot v5 component <component> auth`
  - `ctgbot v5 component <component> auth status`
  - `ctgbot image build`

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
- `internal/containerassets`: embedded Docker build context tarball source
- `internal/ctgbotimage`: standalone image build helpers
- `internal/broker`, `internal/component`, `internal/system`, `internal/runtime`: the real runtime path
- `docker/Dockerfile`: source Dockerfile for the embedded image build context

## Main Commands

- `go run ./cmd/ctgbot v5 run`
  Runs the live `v5` runtime.

- `go run ./cmd/ctgbot v5 workspace list`
  Lists configured `v5` workspaces.

- `go run ./cmd/ctgbot v5 component list`
  Lists registered `v5` components.

- `go run ./cmd/ctgbot v5 component codex auth`
  Runs the component-scoped auth flow for the default Codex registration.

- `go run ./cmd/ctgbot v5 component codex auth status`
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

## v5 Model

The important `v5` concepts are:

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
The canonical CLI form is `v5 component <component> ...`; the older
`v5 component auth <component>` and `v5 component auth-status <component>`
forms remain as compatibility aliases.

For the more detailed design model, see:

- [internal/README.md](/Users/bart/src/go/ctgbot/internal/README.md)
- [V5DESIGN.md](/Users/bart/src/go/WORKSPACE-DOCS/ctgbot/V5DESIGN.md)

## Known Operational Notes

- Codex component auth uses the callback relay opened by the runtime.
  The default callback port is `127.0.0.1:1455`.

- After changing `docker/Dockerfile` or anything under the embedded build context, regenerate and rebuild:
  1. `go run ./cmd/ctgbot go-generate`
  2. `go run ./cmd/ctgbot image build --no-cache`

- The process command surface is global bot-process control, not a component surface.
  In chat it appears as:
  - `/install`
  - `/upgrade`
  - `/quit`
  - `/process ...`

## Editing Guidance

- Keep `cmd/` files focused on `clir` routing and small orchestration.
- Keep standalone helper logic in honest packages like `internal/ctgbotimage`.
- Keep runtime behavior in `internal/broker`, `internal/component`, `internal/system`, and `internal/runtime`.
- Keep message transport types in `internal/message`.
- Keep config/state access in `internal/appstate`.
- Prefer updating the embedded build-context source files and then regenerating `internal/containerassets/assets/src.tar.gz`.
- Do not commit local runtime data from `./.ctgbot/`.

## Good First Checks

If something breaks, check these first:

1. `go run ./cmd/ctgbot config`
2. `go run ./cmd/ctgbot v5 component list`
3. `go run ./cmd/ctgbot v5 component codex auth status`
4. `go run ./cmd/ctgbot image build --no-cache`
5. whether the Docker image is stale after changing Dockerfile or embedded assets
