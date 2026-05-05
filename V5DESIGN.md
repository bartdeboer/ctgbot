# V5 design

This document captures the intended `v5` model so we can refactor toward it
without drifting.

It is intentionally opinionated. When current code differs from this document,
the document describes the target direction unless stated otherwise.

## Design goals

`v5` should keep four concepts separate:

- workspace: where work happens
- component home/state: where a component keeps auth, config, and local state
- runtime: how a component executes
- chat binding: which components play which roles in a chat

The older `profile` concept bundled some of these together. The `v5` direction
is to separate them cleanly.

## Core model

### Workspace

A workspace is a named host path.

Examples:

- `work` -> `/work/workspace`
- `personal` -> `/workspace`

Workspaces are configured in `config.json`.

They do not define runtime type.
They do not own component state.
They are just named working directories.

### Component registration

A component registration defines a reusable component instance.

It should carry:

- `Type`
- `Name`
- `Runtime`
- optional `HomePath`
- `Enabled`
- `IsDefault`

Examples:

- `codex`
- `codex/work`
- `codex/personal`
- `telegram/bot1`
- `gmail/work`
- `gmail/personal`

Runtime belongs here because it is part of how this component instance is meant
to execute.

Home/state belongs here because different component registrations may need:

- separate homes by default
- explicitly shared homes as an escape hatch

### Chat

A chat defines routing and work context.

It should carry:

- `Label`
- optional `Workspace`
- `Enabled`

The workspace is a property of the chat, not of the component.

This means a chat says:

- "work in workspace `work`"

while component bindings say:

- "use `codex/work` as the agent"
- "use `telegram/bot1` as source and relay"

### Chat component binding

A chat-component binding defines role only.

It should carry:

- `ChatID`
- `ComponentID`
- `Role`
- `ExternalChatID` when needed for source or relay
- `Enabled`

It should not normally carry runtime.

Runtime lives on the component registration.

## Config shape

The target user config shape is:

```json
{
  "workspaces": {
    "work": {
      "path": "/work/workspace"
    },
    "personal": {
      "path": "/workspace"
    }
  },
  "telegram": {
    "token": "***",
    "operators": [1234, 6789]
  }
}
```

Notes:

- `workspaces` replaces the older `profiles` concept.
- a workspace stores only a path
- runtime is configured on component registrations, not in workspace config

## Filesystem layout

### Component homes

Default component homes live under:

```text
./.ctgbot/components/<type>/<name>
```

Examples:

```text
./.ctgbot/components/codex/codex
./.ctgbot/components/codex/work
./.ctgbot/components/telegram/bot1
./.ctgbot/components/gmail/personal
```

If a component is registered with `--home <path>`, that explicit path is used
instead of the derived default path.

### Workspaces

Named workspaces come from config and may point anywhere on the host:

```text
/work/workspace
/workspace
```

### Default chat-local workspace

If a chat does not have a named workspace assigned, it should use a default
chat-local workspace:

```text
./.ctgbot/chats/<chatID>/workspace
```

This is intentionally chat-scoped, not thread-scoped.

The reason is simple:

- multiple threads in the same chat should normally act on the same workspace
- thread continuity is a broker concern, not a filesystem layout concern

## Runtime model

Runtime answers:

- should this component execute via Docker?
- should this component execute locally?
- should this component later execute via some other backend?

Current expected enum values:

- `docker`
- `local`

Runtime is configured per component registration.

Examples:

- `codex/work` with runtime `docker`
- `codex/personal` with runtime `local`

This means the same named workspace may be used by different component
registrations with different runtimes if desired.

If the user truly wants two variants of one component against the same
workspace, they should register two component instances explicitly rather than
overloading chat bindings with runtime overrides.

## CLI examples

### Register workspaces

These commands are illustrative of the target model:

```bash
ctgbot v5 workspace set work --path /work/workspace
ctgbot v5 workspace set personal --path /workspace
ctgbot v5 workspace list
```

### Register components

Default registration uses the standard component home:

```bash
ctgbot v5 component register codex --runtime docker
```

This implies:

- type: `codex`
- name: `codex`
- runtime: `docker`
- home: `./.ctgbot/components/codex/codex`

Named registration uses the standard named component home:

```bash
ctgbot v5 component register codex work --runtime docker
ctgbot v5 component register codex personal --runtime local
ctgbot v5 component register telegram bot1
ctgbot v5 component register gmail personal
```

This implies:

- `codex/work` -> `./.ctgbot/components/codex/work`
- `codex/personal` -> `./.ctgbot/components/codex/personal`
- `telegram/bot1` -> `./.ctgbot/components/telegram/bot1`
- `gmail/personal` -> `./.ctgbot/components/gmail/personal`

Explicit-home registration overrides the derived component home:

```bash
ctgbot v5 component register codex work --runtime docker --home /srv/codex-work
ctgbot v5 component register gmail personal --home /srv/gmail-shared/personal
```

This is the escape hatch for:

- reusing an existing state folder
- deliberately sharing state
- placing sensitive state somewhere specific on disk

### Assign workspace to a chat

Target command shape:

```bash
ctgbot v5 chat set-workspace <chat-id> work
```

Or clear it and fall back to the default chat-local workspace:

```bash
ctgbot v5 chat clear-workspace <chat-id>
```

### Bind components to a chat

Examples:

```bash
ctgbot v5 chat <chat-id> component add source telegram/bot1 --external-chat-id <provider-chat-id>
ctgbot v5 chat <chat-id> component add relay telegram/bot1 --external-chat-id <provider-chat-id>
ctgbot v5 chat <chat-id> component add agent codex/work
ctgbot v5 chat <chat-id> component add command process
```

## Chat setup requirements

To make a chat operational, the user must set up:

1. at least one source component binding
2. at least one relay component binding
3. at least one agent component binding if the chat should run agents
4. optionally one or more command component bindings
5. optionally a named workspace

If no workspace is assigned:

- the chat uses `./.ctgbot/chats/<chatID>/workspace`

If a named workspace is assigned:

- the chat uses that configured workspace path

## Example setups

### One Docker-backed work chat

Config:

```json
{
  "workspaces": {
    "work": {
      "path": "/work/workspace"
    }
  }
}
```

Registration:

```bash
ctgbot v5 component register telegram bot1
ctgbot v5 component register codex work --runtime docker
ctgbot v5 component register process
```

Chat setup:

```bash
ctgbot v5 chat create work
ctgbot v5 chat set-workspace <chat-id> work
ctgbot v5 chat <chat-id> component add source telegram/bot1 --external-chat-id <telegram-chat>
ctgbot v5 chat <chat-id> component add relay telegram/bot1 --external-chat-id <telegram-chat>
ctgbot v5 chat <chat-id> component add agent codex/work
ctgbot v5 chat <chat-id> component add command process
```

Result:

- the chat works in `/work/workspace`
- the Codex component runs via Docker
- the Codex component keeps its own state under its component home

### Personal local chat with default workspace fallback

Registration:

```bash
ctgbot v5 component register telegram bot1
ctgbot v5 component register codex personal --runtime local
ctgbot v5 component register process
```

Chat setup:

```bash
ctgbot v5 chat create personal
ctgbot v5 chat <chat-id> component add source telegram/bot1 --external-chat-id <telegram-chat>
ctgbot v5 chat <chat-id> component add relay telegram/bot1 --external-chat-id <telegram-chat>
ctgbot v5 chat <chat-id> component add agent codex/personal
ctgbot v5 chat <chat-id> component add command process
```

Result:

- the chat uses `./.ctgbot/chats/<chatID>/workspace`
- the Codex component runs locally

### Shared explicit component state

Registration:

```bash
ctgbot v5 component register gmail work --home /srv/mail/work
ctgbot v5 component register gmail personal --home /srv/mail/personal
```

This gives the user explicit control over the Gmail state locations while still
keeping chat routing and workspaces independent.

## Why runtime is on the component

Runtime is intentionally configured on the component registration instead of on
the chat binding.

Reasons:

- a component registration is a reusable instance definition
- runtime is part of how that instance is meant to execute
- chat bindings should stay focused on routing roles
- the current `v5` component-loading seams already fit this shape much better
  than per-binding runtime overrides

If a user wants two variants of one component, the system should prefer two
explicit component registrations over hidden chat-level runtime overrides.

Examples:

- `codex/work-docker`
- `codex/work-local`

That is clearer than binding the same component differently in different chats.

## Why workspace is on the chat

Workspace is intentionally configured on the chat.

Reasons:

- the workspace is part of the conversation context
- different chats may act on different work trees
- multiple threads in one chat should normally share the same working
  directory
- source and relay components do not inherently own the work tree

This keeps the model readable:

- component says how it runs and where its state lives
- chat says where work happens

## Migration direction

When refactoring current code toward this design, the intended direction is:

1. rename `profiles` to `workspaces`
2. move workspace assignment onto chats
3. replace component `Profile` with component `Runtime`
4. add optional component `HomePath`
5. change default runtime workspace fallback from thread-scoped to chat-scoped

The goal is not to preserve the older bundled `profile` semantics. The goal is
to end up with a cleaner and more orthogonal model.
