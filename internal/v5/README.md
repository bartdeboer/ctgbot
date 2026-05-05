# v5 groundwork

`v5` is a reset toward a flatter, more human-readable model.

The current direction is:

- workspaces come from config
- components are global registrations
- components carry runtime and optional home overrides
- chats carry workspace context
- broker wires source, relay, agent, and command roles per chat

## Core model

The main concepts are:

- `Workspace`
- `Runtime`
- `Component`
- `Chat`
- `ChatComponent`
- `ThreadComponentMapping`
- `Broker`

Everything else should support those concepts directly instead of hiding them
behind managers and option structs.

## Workspace flow

Workspaces are loaded from config.

Each workspace defines:

- `path`

A workspace is just a named host path.

Chats may point at a named workspace.

If a chat does not have a named workspace, `v5` falls back to a chat-local
workspace:

- `.ctgbot/chats/<chatID>/workspace`

This default is intentionally chat-scoped, not thread-scoped.

## Runtime flow

Runtime is configured per component registration.

Examples:

- `codex/work` with runtime `docker`
- `codex/personal` with runtime `local`

At startup we build runtime factories by kind:

- `docker`
- `local`

These runtime factories are wired and ready, but they do not start containers
or processes yet.

Actual execution only starts when:

- a component runs auth
- an agent handles a turn

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

When a component is resolved:

1. load the registration row
2. find the runtime for that component
3. derive the component home from the registration
4. call the constructor for that component type

The constructor signature stays intentionally direct and explicit.

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
- component home conventions beyond the explicit runtime/component seams

## Current status

`v5` currently proves the flatter architecture:

- config-backed workspaces
- runtime chosen per component registration
- explicit component home handling
- component resolution with runtime injection
- broker routing with thread-component mapping
- hostbridge command support in the runtime path

What is intentionally unfinished:

- real `local` runtime execution
- broader deploy polish after the first live `v5` rollout

For the more opinionated target model and CLI examples, see:

- [V5DESIGN.md](/Users/bart/src/go/ctgbot/V5DESIGN.md)
