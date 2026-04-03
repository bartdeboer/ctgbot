# codextgbot Agent Guide

## Purpose

`codextgbot` is a Telegram bot that runs Codex inside Docker.

The project has three major responsibilities:

- run Codex in a reusable standalone Docker profile via `codextgbot codex`
- run Codex conversations per Telegram chat/thread via `codextgbot telegram monitor`
- provide a host-command bridge via `hostbridge`, `hostbridge-controller`, and `tcphostbridge`

## Repo Shape

- `cmd/codextgbot`: main CLI entrypoint
- `cmd/hostbridge`: container-side hostbridge client
- `cmd/hostbridge-controller`: host-side socket controller
- `cmd/tcphostbridge`: host-side TCP controller
- `cmd/pack`: generates the embedded Docker build context tarball
- `internal/botengine`: Telegram bot, config, session runtime, Codex runner, image builder
- `internal/hostbridge`: shared hostbridge protocol and controller runtime
- `internal/containerassets`: embeds `src.tar.gz` for Docker image builds
- `docker/Dockerfile`: source Dockerfile for the embedded image build context

## Main Commands

- `go run ./cmd/codextgbot codex`
  Runs normal Codex inside the bot Docker image.

- `go run ./cmd/codextgbot codex signin`
  Runs containerized Codex login and persists auth on the host under `~/.codextgbot/.codex`.

- `go run ./cmd/codextgbot codex status`
  Shows login status using the standalone Codex home.

- `go run ./cmd/codextgbot telegram monitor`
  Starts the Telegram bot loop.
  This now also starts an in-process TCP hostbridge controller for Telegram conversations.

- `go run ./cmd/tcphostbridge`
  Starts the hostbridge controller over TCP.

- `go run ./cmd/codextgbot image build --no-cache`
  Rebuilds the Docker image.

- `go run ./cmd/codextgbot go-generate`
  Runs `go generate ./internal/containerassets`.

- `go run ./cmd/codextgbot config`
  Shows current config and discovered Telegram chats.

## Runtime Layout

There are three important state locations:

- `~/.codextgbot/.codex`
  Standalone Codex home used by `codextgbot codex` and `codextgbot codex signin`.
  This is a full Codex home, not just auth storage.

- `./.codextgbot`
  Local bot control state for the current project directory.
  Contains `config.json` and `codextgbot.db`.

- `./chats/<chat_id>-<thread_id>`
  Per Telegram chat runtime.
  Each chat gets:
  - `.codex/` as its Codex home
  - `workspace/` as its default writable project directory
  - `logs/` for chat-local logs if needed

`chats/` and `./.codextgbot/` are runtime data and are gitignored.

## Telegram Model

- All chats are disabled by default until explicitly enabled in config.
- First contact from a chat records the chat ID into local config.
- Enable a chat with:
  - `go run ./cmd/codextgbot config --enable-chat-id <chat-id>`
- `/new` starts a fresh Docker container for that Telegram chat/thread.
- `/new` does not wipe the chat `.codex` home or `workspace/`.
- The default Telegram workspace is `./chats/<chat_id>-<thread_id>/workspace`.
- `workspace/` is initialized as a tiny Git repo so Codex treats it as a writable project.

## Codex Sandbox Notes

Telegram conversations currently rely on:

- Docker as the outer sandbox boundary
- Codex `workspace-write` mode inside the container
- explicit chat `config.toml` written into `./chats/<id>/.codex/config.toml`
- `writable_roots = ["/workspace"]`
- TCP hostbridge access via `HOSTBRIDGE_ADDR=host.docker.internal:4567`
- `network_access = true` so the TCP hostbridge path can be reached from Telegram sessions
- the container installs `hostbridge` in `/usr/bin/hostbridge` because that location works with Codex's shell runner

The conversation container currently runs with:

- `--security-opt seccomp=unconfined`

This was needed to get Codex file writes working in the Dockerized Telegram setup.

## Known Quirks

- Codex may create an empty `workspace/.codex` file during use.
  Treat this as an upstream Codex bug for now, not project state.
  There is a recent upstream issue for similar behavior:
  - `openai/codex#16088`

- After changing `docker/Dockerfile` or anything under the embedded build context, regenerate and rebuild:
  1. `go run ./cmd/codextgbot go-generate`
  2. `go run ./cmd/codextgbot image build --no-cache`

- `codex signin` depends on the host-side callback relay implemented in `internal/botengine/signin_relay.go`.
  The callback port is fixed at `127.0.0.1:1455`.

## Editing Guidance

- Keep `cmd/` files focused on `clir` routing.
- Put logic in `internal/botengine` unless there is a strong reason to split further.
- Prefer updating the embedded build-context source files and then regenerating `internal/containerassets/src.tar.gz`.
- Do not commit runtime chat data from `chats/` or local control data from `./.codextgbot/`.

## Good First Checks

If something breaks, check these first:

1. `go run ./cmd/codextgbot config`
2. `go run ./cmd/codextgbot codex status`
3. `go run ./cmd/codextgbot image build --no-cache`
4. whether the chat is enabled
5. whether the Docker image is stale after changing the Dockerfile or embedded assets
