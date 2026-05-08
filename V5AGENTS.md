# V5 Operational Notes

This file is a short handoff note for host-side deployment and operational work
around the `v5` runtime.

`v5` is now the live ctgbot path. The old `v1` runtime path has been retired
from this repo.

## Core Shape

The live bot runs through:

- `ctgbot v5 run`

The important persistent state lives under the runtime root, typically:

- `./.ctgbot/config.json`
- `./.ctgbot/ctgbot.db`
- `./.ctgbot/components/<type>/<name>/`
- `./.ctgbot/chats/<chatID>/workspace`
- `./.ctgbot/hostbridge/`

## Current Command Surfaces

The important global operational commands are:

- `process install`
- `process upgrade`
- `process quit`
- `v5 component <component> auth`
- `v5 component <component> auth status`

Convenience aliases still exist for:

- `install`
- `upgrade`
- `quit`

Those commands are available both:

- from the standalone CLI
- from the in-chat process command surface

## Typical Bring-Up

For a fresh runtime root, the normal shape is:

1. Ensure `./.ctgbot/config.json` exists.
2. Configure Telegram token and operators.
3. Optionally configure named workspaces.
4. Register components.
5. Authenticate Codex if needed.
6. Create chats and bind components.
7. Start the bot with:
   - `ctgbot v5 run`

## Component Homes

Static per-component config now belongs in the component home:

- `runtime.json`
- `component.json`

Examples:

- Codex runtime image / GPU settings
- llama.cpp backend image / GPU settings
- Codex default model / reasoning defaults
- llama.cpp model path / backend config
- component-scoped auth material such as Codex `auth.json`

Mutable per-thread component settings belong in the database through
`ThreadComponentState`, not in the component home files.
The canonical CLI form is `v5 component <component> ...`; the older
`v5 component auth <component>` and `v5 component auth-status <component>`
forms remain as compatibility aliases.

## Deployment Notes

- `ctgbot image build --no-cache` rebuilds the shared Codex image.
- `v5 component codex auth` authenticates the default Codex registration.
- `v5 component codex auth status` checks authentication state for that registration.
- `process upgrade` runs:
  - `git pull --ff-only`
  - `go generate ./internal/containerassets`
  - `go install ./cmd/ctgbot ./cmd/hostbridge`
  - `ctgbot image build --no-cache`
- `process quit` requests a clean process shutdown so the supervisor or operator
  can restart the bot.

## Guardrails

- Prefer working through the `v5` component graph instead of adding new
  standalone flows.
- Keep static component settings in component homes.
- Keep mutable thread-scoped settings in the `v5` DB model.
- Keep hostbridge command surfaces aligned with the current bound component.

## Design Reference

For the fuller design model, see:

- `/Users/bart/src/go/ctgbot/internal/README.md`
- `/Users/bart/src/go/WORKSPACE-DOCS/ctgbot/V5DESIGN.md`
