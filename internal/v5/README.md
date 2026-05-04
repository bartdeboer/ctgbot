# v5 groundwork

`v5` is a reset toward a flatter, more human-readable model.

The goal is simple:

- profiles come from config
- runtimes are built from profiles
- registered components come from the database
- components are constructed with their runtime
- broker wires inbound, agent, and outbound components per chat

## Core model

The main concepts are:

- `Profile`
- `Runtime`
- `Component`
- `ChatComponent`
- `ThreadComponentMapping`
- `Broker`

Everything else should support those concepts directly instead of hiding them
behind managers and option structs.

## Profile flow

Profiles are loaded from config.

Each profile defines:

- `runtime`
- optional `home_path`

The resolved profile gives:

- profile name
- runtime kind
- profile root on disk

## Runtime flow

At startup we build one runtime instance per profile.

Examples:

- `docker(default)`
- `docker(work)`
- `local(personal)`

These runtime objects are wired and ready, but they do not start containers or
processes yet.

Actual execution only starts when:

- a component runs auth
- an agent handles a turn

## Component flow

Registered components come from the database and carry:

- `Type`
- `Name`
- `Profile`

When a component is resolved:

1. load the registration row
2. find its profile
3. find the runtime for that profile
4. derive the component home from that runtime
5. call the constructor for that component type

The constructor signature is intentionally direct and explicit.

## Broker flow

Broker owns:

- inbound routing
- chat binding lookup
- thread mapping
- command surface aggregation
- handing turns to agent components
- relaying outbound payloads

Broker does not own:

- Docker internals
- local process internals
- component home conventions
- auth implementation details

## Current status

`v5` currently proves the flatter architecture:

- direct component constructors
- config-backed profiles
- runtime instances per profile
- component resolution with runtime injection
- broker routing with thread-component mapping

What is intentionally unfinished:

- real `local` runtime execution
- privileged message-command actor resolution for operator-only commands like `/quit`
- production rollout and migration from the older live runtime
