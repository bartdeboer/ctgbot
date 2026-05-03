# v4 groundwork

This folder starts the next refinement of the `ctgbot` architecture.

The main shift from `v3` is that component execution policy is no longer
treated as part of broker state. Instead:

- the database stores registered components and chat bindings
- config stores profiles
- profiles choose a runtime backend like `docker` or `local`
- component resolution wires the matching runtime into the live component
- actual execution remains lazy and only starts during auth or a turn

## Core concepts

### Component type

Defined in code through the component registry.

Examples:

- `telegram`
- `gmail`
- `codex`

These stay code-only and are not persisted as rows.

### Registered component

Persisted in `coremodel.Component`.

A registered component is a concrete configured instance:

- `telegram`
- `telegram/bot2`
- `gmail`
- `gmail/work`
- `codex`
- `codex/personal`

Each registered component now also carries a `Profile`.

The profile does not mean "start a runtime now". It means:

- which runtime driver should be used later
- which profile root should be used for long-lived component files

### Profiles

Profiles live in config, not in the database.

Current settings are modeled in `profiles.Settings`:

- `runtime`
- `home_path`

Examples:

```json
{
  "profiles": {
    "default": { "runtime": "docker" },
    "work": { "runtime": "docker" },
    "personal": { "runtime": "local", "home_path": "/some/path" }
  }
}
```

If `home_path` is not configured, the default profile root is:

- host: `./.ctgbot/profiles/<profile>`

### Component home

Every registered component gets a conventional home under its profile root:

- host: `<profile root>/components/<type>/<name>`
- container: `/profile/components/<type>/<name>`

This is where auth files, config, cache, and other long-lived component state
live.

### Runtime wiring

When the runtime resolves a registered component into a live instance, it:

1. loads the component row
2. resolves the configured profile
3. derives the component home under that profile
4. resolves the runtime backend from the profile
5. instantiates the component with that runtime

That wiring step does not start execution yet.

Actual execution still happens lazily:

- auth flow -> `Runtime.StartAuth(...)`
- agent turn -> `Runtime.StartTurn(...)`

This keeps broker out of runtime internals while still giving agent components
the execution substrate they need.

### Chat component binding

Persisted in `coremodel.ChatComponent`.

Chats bind registered components by role:

- `source`
- `relay`
- `agent`
- `command`

Broker sees resolved components and capabilities. It does not need to know how
those components are executed.

### Thread component mapping

Persisted in `coremodel.ThreadComponentMapping`.

This remains the canonical mapping between:

- a ctgbot `ThreadID`
- a registered `ComponentID`
- that component's own thread/session identifier

The first-class broker seam is `ThreadComponentMapper`, which:

- resolves or creates internal threads for inbound component events
- looks up mapped component thread ids for outbound relay and agent resume
- binds component thread ids after first use

## Broker responsibilities

The broker flow is still:

1. start enabled inbound sources
2. receive an inbound event from a registered source component
3. resolve the bound chat from `(component, external chat id)`
4. resolve or create the thread from `(chat binding, component thread id)`
5. persist inbound messages and artifacts
6. resolve the chat runtime:
   - agent components
   - relay components
   - command surfaces
   - component homes
7. run bound agents
8. persist and relay outbound payloads through bound relay components

Broker should not manage:

- Docker details
- local process details
- hostbridge wiring
- component auth internals
- profile home conventions

Those belong to the runtime layer around the broker.

## Current runtime seam

`runtime.Runtime` currently owns the system-level helpers around:

- opening storage and config-backed profile managers
- ensuring registered components exist
- binding chats to components by role
- resolving registered components into live instances
- launching component auth through the component's configured runtime

`execution.Resolver` is the first runtime wiring seam. Today it provides:

- `docker` runtime objects
- `local` runtime objects

The `local` runtime is still a placeholder.
The `docker` runtime already owns sandbox creation, but hostbridge injection has
not been moved there yet.

## Current status

`v4` currently proves:

- registered components persist a profile name
- profiles live in config and can select a runtime driver
- component homes are derived under the profile root
- runtime selection happens during component resolution
- agent components can receive a runtime dependency instead of constructing
  sandbox details themselves
- broker and thread-component mapping stay intact

The intentionally unfinished areas are:

- moving hostbridge injection fully behind the runtime implementation
- building the local runtime backend for real
- adding `v4` CLI routes
- porting real Telegram and Codex behavior fully onto the new runtime seam
