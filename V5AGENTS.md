# V5 deploy handoff

This file is for the `v2` authoring / coding agent that will do the actual
host-side deployment work outside the current workspace sandbox.

The goal is to stand up `v5` next to the live `v1` bot without disturbing the
existing `v1` deploy.

## Context

- `v1` stays running from `/bots/ctgbot-01`
- `v5` should run from `/workspace/run/ctgbot-02`
- the current `v2` instance will be shut down and its Telegram token reused for
  `v5`
- supervisor is used to keep the process alive and to relaunch it after manual
  restart

## Important current `v5` expectations

`v5` now expects this local state layout inside the run dir:

- config store: `.ctgbot/config.json`
- state root default: `.ctgbot`
- default database path: `.ctgbot/ctgbot.db`
- component homes default under: `.ctgbot/components/<type>/<name>`
- default chat-local workspaces under: `.ctgbot/chats/<chatID>/workspace`
- hostbridge TLS/server root default under: `.ctgbot/hostbridge`

Named workspaces are configured in config using:

- `workspaces`

Example:

```json
{
  "workspaces": {
    "work": {
      "path": "/work/workspace"
    }
  },
  "telegram": {
    "token": "...",
    "operators": [123456]
  }
}
```

`v5` DB tables are unversioned. The schema is isolated by using a separate DB
file, not by table prefixes.

Docker runtime container names intentionally still include a `v5` marker to
avoid collisions with `v1` and `v2` while multiple runtimes share one Docker
daemon.

## Current known limitation

Do not rely on `/install` for remote operator workflow yet.

`/quit` now works through the operator actor path, but first deploy bring-up
should still prefer simplicity over operator parity.

For the first deploy, assume:

- manual or supervisor-managed restart is okay
- getting the bot up cleanly is more important than polishing every workflow

## Preferred first deploy shape

Keep the first live shape simple:

- one Telegram component for both source and relay
- one Codex component as agent
- one process component bound as command surface
- a Docker runtime for the Codex component

That means the likely first component set is:

- `telegram`
- `codex`
- `process`

and optionally one named workspace such as:

- `work`

## Suggested host-side sequence

From `/workspace/run/ctgbot-02`:

1. Ensure the run directory exists and is clean enough for a first deploy.
2. Ensure `.ctgbot/config.json` exists.
3. Configure Telegram token and operators in config.
4. Optionally configure a named workspace:
   - `ctgbot v5 workspace set work --path /work/workspace`
5. Register components:
   - `ctgbot v5 component register telegram --telegram-token <token>`
   - `ctgbot v5 component register codex --runtime docker --codex-image ctgbot-codex:latest`
   - `ctgbot v5 component register process --runtime local`
6. Authenticate Codex:
   - `ctgbot v5 component auth codex`
7. Create a chat:
   - `ctgbot v5 chat create main`
8. Optionally assign a named workspace to that chat:
   - `ctgbot v5 chat <chatID> workspace set work`
9. Bind components to that chat:
   - source -> `telegram`
   - relay -> `telegram`
   - agent -> `codex`
   - command -> `process`
10. Start the bot with supervisor using:
    - `ctgbot v5 run`

Use explicit `--state-root` or `--db-path` only if the host deploy layout truly
needs it. The new defaults should be preferred.

## Smoke checks

After the first bring-up, verify:

1. `ctgbot v5 workspace list` shows any named workspaces you configured.
2. `ctgbot v5 component list` shows the expected components with runtimes.
3. `ctgbot v5 chat list` shows the created chat and its workspace assignment.
4. `ctgbot v5 chat <chatID> component list` shows source, relay, agent, and
   command bindings.
5. A normal Telegram message reaches Codex and gets a reply.
6. A slash command like `/process quit` is intercepted as a command and does
   not go into the normal thread history.
7. If the agent sends media through hostbridge, the outbound relay still works.

## Guardrails

- Do not touch `/bots/ctgbot-01` while bringing up `v5`.
- Do not rename or remove the live `v1` supervisor entry during the first
  deploy.
- Prefer additive setup in `/workspace/run/ctgbot-02`.
- Keep Docker container-name version markers as-is for now.
- If a deploy-time issue appears, prioritize restoring a working `v5 run` loop
  over cleaning up polish.

## Design reference

For the intended long-term `v5` shape, see:

- [V5DESIGN.md](/Users/bart/src/go/ctgbot/V5DESIGN.md)
