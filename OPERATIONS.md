# Operational Notes

This file is a short handoff note for host-side deployment and operational work
around the ctgbot runtime.

## Core Shape

The live bot runs through:

- `ctgbot run`

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
- `component <component> auth`
- `component <component> auth status`

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
   - `ctgbot run`

## Gmail OAuth

Gmail auth uses a deployment-provided Google OAuth Desktop client config at:

```text
<state-root>/google/oauth_client.json
```

Each Gmail component registration stores its own mailbox token and poll state in
its component home, for example:

```text
<state-root>/components/gmail/work/token.json
<state-root>/components/gmail/personal/token.json
```

Typical flow:

```bash
ctgbot component register gmail/work
ctgbot component gmail/work auth
ctgbot chat <chatID> component add source gmail/work
```

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
The canonical CLI form is `component <component> ...`.

## Deployment Notes

- `ctgbot image build --no-cache` rebuilds the shared Codex image.
- `component codex auth` authenticates the default Codex registration.
- `component codex auth status` checks authentication state for that registration.
- `process upgrade` runs:
  - `git pull --ff-only`
  - `go generate ./internal/buildassets`
  - `go install ./cmd/ctgbot ./cmd/hostbridge`
  - `ctgbot image build --no-cache`
- `process quit` requests a clean process shutdown so the supervisor or operator
  can restart the bot.

## Guardrails

- Prefer working through the component graph instead of adding new standalone flows.
- Keep static component settings in component homes.
- Keep mutable thread-scoped settings in the runtime DB model.
- Keep hostbridge command surfaces aligned with the current bound component.

## Design Reference

For the fuller design model, see:

- `/Users/bart/src/go/ctgbot/internal/README.md`
- `/Users/bart/src/go/WORKSPACE-DOCS/ctgbot`
