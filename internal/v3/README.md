# v3 groundwork

This folder lays down the core shape for a plugin-oriented `ctgbot` runtime.

The intent is to keep the roles clean:

- code plugins define **component types**
- registered/configured instances become **components**
- chats bind components by **role**
- the broker sees resolved component instances and capabilities
- component homes and auth live outside the broker

## Core concepts

### Component type
Defined in code through a factory in the component registry.

Examples:

- `telegram`
- `gmail`
- `codex`

These are not persisted as database rows.

### Registered component
Persisted in `coremodel.Component`.

This is the configured instance of a component type:

- `telegram`
- `telegram/bot2`
- `gmail`
- `gmail/work`

The default registered component for a type uses the type name as the
registration name, so `telegram` resolves to the default registered
`telegram/telegram` component internally.

### Component home
Every registered component gets a conventional home:

- host: `./.ctgbot/components/<type>/<name>`
- container: `/components/<type>/<name>`

This is where auth files, config, cache, and other long-lived component state
can live.

### Chat component binding
Persisted in `coremodel.ChatComponent`.

A chat binds registered components by role:

- `source`
- `relay`
- `agent`
- `command`

This keeps capability decisions out of the broker's storage model while still
letting the broker route by role at runtime.

### Thread component mapping
Persisted in `coremodel.ThreadComponentMapping`.

This stores the canonical mapping between:

- a ctgbot `ThreadID`
- a registered `ComponentID`
- that component's own thread/session identifier

The important design choice in `v3` is that provider-facing IDs do not live on
`Chat` or `Thread` globally. They live on:

- `ChatComponent.ExternalChatID`
- `ThreadComponentMapping.ComponentThreadID`

This is now a first-class broker seam through `ThreadComponentMapper`, which:

- resolves or creates ctgbot threads for inbound component events
- looks up component thread/session ids for outbound relay or agent resume
- binds new component thread/session ids after first use

That allows one chat to have multiple inbound/outbound providers, and lets
agent components like Codex map their own provider session ids through the same
system seam.

## Broker flow

The broker flow is:

1. start enabled inbound sources
2. receive an inbound event from a registered source component
3. resolve the bound chat from `(component, external chat id)`
4. resolve or create the thread from `(chat binding, component thread id)`
5. persist inbound message + artifacts
6. resolve the chat runtime:
   - agent components
   - relay components
   - command surfaces
   - component homes
7. run all bound agents
8. persist and relay outbound payloads through bound relay components

The broker should not manage:

- component auth flows
- home directory conventions
- sandbox profile mounts
- component registration policy

Those belong to the runtime/control seams around it.

## Current runtime seam

`runtime.Runtime` currently owns the system-level helpers around registration
and binding:

- resolve component refs
- ensure registered components exist
- bind chats to components by role
- launch component auth against a prepared component home

This is meant as a practical control seam, not as a place to smuggle broker
policy back in.

## Current status

`v3` now already has:

- CLI routes in `cmd/ctgbot/v3_routes.go`
- GORM-backed storage and runtime open helpers
- first Telegram and Codex component adapters
- thread-scoped workspaces and component homes
- broker-owned inbound/outbound routing through component roles

The main intentionally unfinished areas are now:

- hostbridge/command-surface execution inside the `v3` agent sandbox
- richer thread/agent selection policy
- migration from `v2`
- broader component coverage beyond the first Telegram/Codex slice
