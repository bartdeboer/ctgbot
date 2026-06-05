# `ctgbot`

`ctgbot` is an open orchestration platform for organic agentic ecosystems.

It gives AI agents persistent identities, isolated runtimes, and authenticated
communication channels — the substrate for agent relationships and specializations
that emerge through use rather than upfront design.

<p align="center">
  <img src="docs/assets/ctgbot-architecture.svg" alt="ctgbot routes messages into isolated agent thread containers" width="900">
</p>

ctgbot is under active development. The architecture is usable today, but command names and component setup may still change.

## Features

- **Persistent agent identities** — agents accumulate context across sessions and develop specializations through use, not role assignments
- **Container isolation per thread** — each agent conversation runs in its own Docker container; experiments stay contained and do not affect the host or other agents
- **Inter-agent messaging** — agents send messages, files, and structured commands to other agent threads via `hostbridge`
- **Organic agent ecosystems** — run a reviewer agent, a coder agent, an email agent, a docs agent side by side; their working relationships emerge from the workflow
- **Explicit trust model** — sources, roles, and command surfaces are declared; no ambient authority; unknown channels are dropped until bound
- **Component model** — agents, sources, relays, and integrations are components attached per chat; mix and match Claude, Codex, Gmail, LLaMA, and custom components
- **Hostbridge** — typed, authenticated command bridge from agent containers to the host; agents cannot reach outside what is explicitly exposed
- **Durable workspaces** — host project directories mounted into containers; conversation history persists across container refreshes
- **Inbound filters** — allowlists and LLM guards attached per source binding
- **Config overlays** — layer deployment defaults under `config.d/` without touching user config
- **Remote agent nodes** — trusted controller/node model for running commands across ctgbot instances over mTLS

## Core model

- Chats receive messages from external channels.
- Threads isolate agent conversations.
- Each thread receives its own runtime sandbox.
- Components provide agents, commands, sources, and integrations.
- Host access is exposed through explicit bridges instead of direct machine access.

## What agents get

A real engineering environment with a persistent place to work.

Each sandboxed thread runs in its own container. Agents can install packages,
run tests, build tools, inspect repositories, create artifacts, run services,
exchange files, communicate with other threads, read message history, and use
restricted `hostbridge` commands.

Agents are not ephemeral task runners. They accumulate context, maintain
conversation history, and develop working knowledge across sessions. A reviewer
agent that has spent weeks reviewing architecture proposals brings genuine
accumulated context — not a blank slate instantiated for each task.

## What humans get

Operators control which channels are trusted, which components are attached,
which workspaces are mounted, and which command surfaces are exposed.

Unknown channels are dropped until explicitly bound.

ctgbot is designed around explicit trust boundaries instead of ambient authority.

## Agent ecosystems

ctgbot does not prescribe how agents relate to each other. Ecosystems emerge
from the workflow. A typical setup might have:

- a **coder agent** (Claude Code or Codex) with access to the repository
- a **reviewer agent** that reads branches and sends feedback via inter-thread messages
- an **email agent** routing inbound mail into threads
- a **docs agent** maintaining documentation in a separate workspace
- a **search agent** indexing and querying conversation history across threads

Agents coordinate by sending messages to each other's threads. The reviewer
asks the coder to push a branch; the coder reports back when it is ready. No
upfront crew definition required — the pattern develops from use.

## Quick start: Telegram + Codex

This quick start sets up Telegram + Codex. Additional components are optional.

### Requirements

- Go 1.24+
- Git
- Docker or Docker Desktop
- A Telegram bot token
- Codex auth

### 1. Install ctgbot

```bash
git clone https://github.com/bartdeboer/ctgbot.git
cd ctgbot

go run ./cmd/ctgbot install
ctgbot version
```

### 2. Create an instance folder

ctgbot stores instance state in the current directory under `.ctgbot/`.

```bash
mkdir -p ~/run/ctgbot-01
cd ~/run/ctgbot-01
```

### 3. Configure basics

A workspace is the host project directory mounted into agent containers.

```bash
ctgbot workspace set default --path /absolute/path/to/workspace

ctgbot config set git.user_name "Your Name"
ctgbot config set git.user_email "you@example.com"
```

### 4. Register Telegram

Set the Telegram bot token in the component profile. `operators` is optional and
marks Telegram user IDs that should be treated as root operators.

```bash
ctgbot component register telegram/telegram --runtime local

mkdir -p .ctgbot/components/telegram/telegram
printf '%s' "$TELEGRAM_BOT_TOKEN" > .ctgbot/components/telegram/telegram/token.txt

cat > .ctgbot/components/telegram/telegram/component.json <<'JSON'
{
  "operators": [123456789]
}
JSON
```

### 5. Register Codex and operator commands

```bash
ctgbot component register codex/codex --runtime docker
ctgbot component register process/process --runtime local
```

### 6. Build runtime images

Docker-based agent components need their runtime image before auth or turns can
start containers.

```bash
ctgbot image list
ctgbot image build --no-cache
```

Agent runtime images include a `ctgbot` user with UID/GID `1000:1000` and
passwordless sudo. Runtime containers default to that user, so agents can install
temporary sandbox tools with `sudo apt-get ...` while keeping workspace files
host-writable. Override the runtime user per component profile when needed:

```json
{
  "uid": 0,
  "gid": 0
}
```

`uid: 0, gid: 0` runs the sandbox as root. Arbitrary non-root UID/GID values are
allowed, but only the baked `1000:1000` user is guaranteed to have sudo.

### 7. Authenticate Codex

```bash
ctgbot component codex/codex auth
ctgbot component codex/codex auth status
```

### 8. Run ctgbot

```bash
ctgbot run
```

Use a process supervisor for production deployments.

### 9. Bind your Telegram chat

Send a message to the Telegram bot from the chat you want to use. Unknown
channels are recorded as dropped until you bind them.

```bash
ctgbot chat dropped
ctgbot chat bind telegram/telegram <telegram_id> "My Chat"
ctgbot chat list

ctgbot chat <chat> workspace set default
ctgbot chat <chat> component add agent codex/codex
ctgbot chat <chat> component add command process/process
ctgbot chat <chat> component list
```

Use Telegram topics to create separate agent conversations, each with its own
thread and sandbox.

## Hostbridge

`hostbridge` is the controlled bridge from an agent container back to ctgbot and
the host. Agents use it to send files, message other threads, read message
history, inspect available components, and run explicit host command aliases.

Inter-agent communication happens through hostbridge. An agent in one container
sends a message directly into another agent's thread:

```bash
hostbridge thread <thread_id> message send <<'EOF'
Review request: feature/my-branch is ready.
EOF
```

### Workspace command aliases

Hostbridge aliases are configured per workspace. Add only commands you are
comfortable letting agents run.

Example `.ctgbot/config.json`:

```json
{
  "workspaces": {
    "default": {
      "path": "/absolute/path/to/workspace",
      "hostbridge": {
        "allowed_commands": {
          "git-fetch": {
            "name": "git",
            "args": ["fetch", "--all", "--prune"],
            "dir": "/absolute/path/to/workspace"
          },
          "git-push": {
            "name": "git",
            "args": ["push"],
            "dir": "/absolute/path/to/workspace",
            "delay": "500ms"
          }
        }
      }
    }
  }
}
```

Agents can then run:

```bash
hostbridge git-fetch
hostbridge git-push
```

## Optional components

### Claude

Minimal Claude chat setup:

```bash
ctgbot component register claude/claude --runtime docker
ctgbot component register process/process --runtime local
ctgbot image build --no-cache
ctgbot component claude/claude auth
ctgbot component claude/claude auth status

ctgbot chat bind telegram/telegram <telegram_id> "Claude #1"
ctgbot chat <chat> workspace set default
ctgbot chat <chat> component add agent claude/claude
ctgbot chat <chat> component add command process/process
ctgbot chat <chat> component list
```

Claude auth runs `claude setup-token` in the component runtime. If it returns a
`CLAUDE_CODE_OAUTH_TOKEN`, store it in the component profile:

```bash
mkdir -p .ctgbot/components/claude/claude
cat > .ctgbot/components/claude/claude/runtime.json <<'JSON'
{
  "env": [
    "CLAUDE_CODE_OAUTH_TOKEN=sk-ant-oat..."
  ]
}
JSON
```

### Gmail

```bash
ctgbot component register gmail/personal --runtime local

mkdir -p .ctgbot/components/gmail/personal
cat > .ctgbot/components/gmail/personal/component.json <<'JSON'
{
  "mailbox_email": "you@example.com"
}
JSON

cp oauth_client.json .ctgbot/components/gmail/personal/oauth_client.json

ctgbot component gmail/personal auth
ctgbot component gmail/personal auth status
ctgbot chat <chat> component add source gmail/personal --external-channel-id you@example.com

# From an agent runtime, send directly through Gmail:
hostbridge gmail/personal message "Monthly report" \
  --to you@example.com \
  --subject "Monthly report" \
  --attach "/workspace/out/report.pdf;type=application/pdf"
```

### Inbound filters

Filters are attached to specific chat/source bindings.

```bash
# Sender allowlist.
ctgbot component register filters/allowlist --runtime local
ctgbot chat <chat> component gmail/personal filter add filters/allowlist

# LLM guard using a completion provider.
ctgbot component register llamacpp/qwen3-q5 --runtime backend
ctgbot component register guard/qwen --runtime local

mkdir -p .ctgbot/components/guard/qwen
cat > .ctgbot/components/guard/qwen/component.json <<'JSON'
{
  "completion": "llamacpp/qwen3-q5"
}
JSON

ctgbot chat <chat> component gmail/personal filter add guard/qwen
```

Allowlist commands:

```text
/allowlist dropped view <drop_id>
/allowlist whitelist sender@example.com
/allowlist whitelist list
/allowlist whitelist remove sender@example.com
```

## Useful chat commands

```text
/status
/version
/quit
/upgrade
/codex status
/codex container refresh
/claude status
/claude container refresh
/thread list
/thread component bind gmail/personal
```

## Useful hostbridge commands

Inside an agent runtime:

```bash
hostbridge help
hostbridge status
hostbridge component list
hostbridge component <component> help
hostbridge thread list
hostbridge thread <thread_id> message send
hostbridge search "<query>"
hostbridge message "hello"
hostbridge message "Report attached" --attach /workspace/out/report.pdf
hostbridge sendstdin
hostbridge sendfile <path>
```

## License

Apache-2.0.
