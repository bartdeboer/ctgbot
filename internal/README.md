# Runtime Groundwork

The current runtime architecture is a reset toward a flatter, more
human-readable model.

The current direction is:

- workspaces come from root config
- workspaces define a host path and hostbridge workflow policy
- component registrations are global reusable instances
- runtime kind lives on the component registration
- static runtime config lives in component-home `runtime.json`
- static component config lives in component-home `component.json`
- chats carry workspace context
- broker wires source, relay, agent, and command roles per chat
- mutable per-thread component settings belong in `ThreadComponentState`

## Core model

The main concepts are:

- `Workspace`
- `Component`
- `Runtime`
- `Chat`
- `ChatComponent`
- `ThreadComponentMapping`
- `ThreadComponentState`
- `Broker`

Everything else should support those concepts directly instead of hiding them
behind managers and option structs.

## Workspace flow

Workspaces are loaded from root config.

Each workspace defines:

- `path`
- optional hostbridge allowed commands

A workspace is the shared work context for a chat. It is not the place for
component auth, model defaults, or Docker image settings.

Chats may point at a named workspace.

If a chat does not have a named workspace, `v5` falls back to a chat-local
workspace:

- `.ctgbot/chats/<chatID>/workspace`

This default is intentionally chat-scoped, not thread-scoped.

## Component flow

Registered components come from the database and carry:

- `Type`
- `Name`
- `Runtime`
- optional `HomePath`

Component homes default to:

- `.ctgbot/components/<type>/<name>`

If `HomePath` is set on the registration, that explicit host path is used
instead.

Each component home may contain:

- auth and local state owned by the component
- `runtime.json` for generic runtime settings such as `image`, `gpus`, and
  `env`
- `component.json` for component-specific static config

When a component is resolved:

1. load the registration row
2. resolve the component home
3. load generic runtime config from `runtime.json`
4. bind the runtime using the registration runtime kind plus that config
5. let the component constructor load its own `component.json`

## Runtime flow

Runtime kind is configured per component registration.

Current expected kinds are:

- `docker`
- `local`

Runtime factories are built by kind:

- `docker`
- `local`

Static runtime settings are not stored on chats. They come from the component
home so different component aliases can carry different execution requirements
without duplicating workspaces.

## Thread component state

Mutable component-specific thread settings belong in `ThreadComponentState`.

Examples:

- Codex model override
- Codex reasoning effort
- future temperature or backend-specific knobs

This is intentionally separate from broker-owned thread/component mapping.

## Broker flow

Broker owns:

- inbound routing
- chat binding lookup
- thread mapping
- command surface aggregation
- handing turns to agent components
- relaying outbound payloads
- resolving the effective workspace for a chat turn

Broker does not own:

- Docker internals
- local process internals
- component auth implementation details
- component-specific profile config schemas
- component home conventions beyond the explicit runtime/component seams

## Current status

`v5` currently proves the flatter architecture:

- config-backed workspaces
- runtime kind chosen per component registration
- explicit component home handling
- component-home `runtime.json` and `component.json` support
- broker routing with thread-component mapping
- `ThreadComponentState` foundation
- Codex thread settings moving out of the generic `Thread` row
- hostbridge command support in the runtime path

What is intentionally unfinished:

- full migration of all component-specific thread state out of older thread
  fields
- broader deploy polish after the first live `v5` rollout

For the more opinionated target model and CLI examples, see:

- `/workspace/WORKSPACE-DOCS/ctgbot/V5DESIGN.md`
