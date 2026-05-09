# Messaging Groundwork

This package is the groundwork for thread-oriented messaging clients.

The main idea is:

- a remote client acts like a regular ctgbot thread participant
- a local hostbridge client can use the same service shape in-proc
- future web clients can use the same HTTP API
- agent-facing command surfaces can be thin adapters over the same service

This is intentionally **not** a component package.

The messaging service is a core ctgbot subsystem. Components may expose it
later, but they should not own the protocol or the core access model.

It should also prefer the existing repo vocabulary where possible:

- reuse `coremodel.Actor` for authenticated caller identity
- reuse `coremodel.ThreadMessage` for persisted thread messages
- introduce new types only where we genuinely need a new service-level shape,
  such as `ThreadSummary`

## Scope

The first stable service shape is intentionally small:

- `ListThreads`
- `ListMessages`
- thread target/ref resolution for adapters

Thread-targeted writes still enter through broker resolved-inbound delivery.
Everything else should build on top of that.

## Layers

The intended layering is:

1. `internal/messaging`
   - actor model
   - concrete read/query domain service
   - thread CRUD/query/ref helpers
   - request/response types
2. `internal/httpapi`
   - HTTP + JSON transport
   - request authentication
   - path/query decoding
3. adapters
   - hostbridge commands
   - `thread` command surface
   - future `ctgbotmessaging` companion CLI

## Actor Model

Messaging clients should be modeled as authenticated actors.

Examples:

- human operator
- local agent
- remote agent
- future web client

The service should not care whether a caller arrived through hostbridge,
HTTP, or a browser session. It should only care about the resolved actor
identity and permissions.

## Current Command Shape

The command shape we are grounding this around is:

- `thread list`
- `thread <threadID> message list [--cursor <cursor>]`
- `thread <threadID> message send <message>`

Those commands should be thin adapters over the same core service used by
the HTTP API.

Today, the local `thread` command surface is already wired through broker
message commands and hostbridge commands. The remote HTTP API is still
groundwork only.

The local split is now:

- `internal/messaging`
  - list/query/cursor/ref logic
- broker
  - resolved inbound delivery
  - message commands
  - agent turn execution
  - relay of thread outputs
- adapters
  - build resolved inbound messages
  - hand them to broker

## Current HTTP Shape

The first HTTP shape is:

- `GET /v1/threads`
- `GET /v1/threads/<threadID>/messages?cursor=<cursor>&limit=<limit>`
- `POST /v1/threads/<threadID>/messages`

The API is deliberately thread-centric rather than component-centric.

## Authentication

The likely first authentication shape is bearer tokens.

That is enough groundwork for:

- local companion clients
- remote LAN clients
- future web session backends

OAuth or browser login can be added later on top of the same service.

## Non-goals for this pass

This groundwork does **not** try to solve:

- push delivery
- reply routing between instances
- mailbox replication
- websocket streaming
- UI design

Those can follow once the core service shape feels right.
