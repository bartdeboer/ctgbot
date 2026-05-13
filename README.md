# ctgbot

## For Humans

ctgbot is an agentic engineering platform optimized for secure agent isolation.

It lets you run coding agents from Telegram, Gmail, and other message sources
without giving those agents ambient host access. ctgbot owns routing,
credentials, workspaces, host actions, runtime images, and chat enablement.
Agent work happens in isolated runtime environments.

## For Agents

ctgbot is an agent-first engineering environment.

You get a real workspace, persistent thread state, a runtime container when your
component needs one, and a narrow `hostbridge` back to the host for approved
actions such as sending files, messaging other threads, and running
operator-approved commands.

```text
human / email / system event
  -> source component
  -> ctgbot broker
  -> chat + thread
  -> agent component
  -> runtime container
  -> hostbridge / artifacts / replies
```

## Installation

### Prerequisites

- Go
- Git
- Docker or Docker Desktop
- Windows, macOS, or Linux
- A Telegram bot token
- Codex auth, if using Codex
- Claude Code auth, if using Claude

### Install

```bash
git clone https://github.com/bartdeboer/ctgbot.git
cd ctgbot

# Build and install ctgbot + hostbridge.
go run ./cmd/ctgbot install

# Confirm the installed binary.
ctgbot version
```

### Create an instance folder

ctgbot stores instance state in the current directory under `.ctgbot/`.

```bash
mkdir -p ~/run/ctgbot-01
cd ~/run/ctgbot-01
```

### Configure a workspace and Git identity

```bash
ctgbot workspace set default --path /workspace
ctgbot workspace list

ctgbot config set git.user_name "Your Name"
ctgbot config set git.user_email "you@example.com"
```

## Register components

### Telegram

```bash
ctgbot component register telegram/telegram --runtime local

install -d -m 700 .ctgbot/components/telegram/telegram
printf '%s' "$TELEGRAM_BOT_TOKEN" > .ctgbot/components/telegram/telegram/token.txt
chmod 600 .ctgbot/components/telegram/telegram/token.txt

cat > .ctgbot/components/telegram/telegram/component.json <<'JSON'
{
  "operators": [123456789],
  "poll_timeout": "60s",
  "debounce_window": "800ms",
  "render_format": "markdown_v2"
}
JSON
```

`render_format` is optional. The default is `markdown_v2`.

### Process commands

```bash
# Provides /quit, /upgrade, /version, and related operator commands.
ctgbot component register process/process --runtime local
```

### SQL command component optional

Only bind this to trusted chats.

```bash
ctgbot component register sql/sql --runtime local
```

### Codex

```bash
ctgbot component register codex/codex --runtime docker
ctgbot component codex/codex auth
ctgbot component codex/codex auth status

cat > .ctgbot/components/codex/codex/component.json <<'JSON'
{
  "model": "gpt-5.3-codex",
  "reasoning_effort": "high",
  "sandbox_mode": "danger-full-access"
}
JSON
```

### Claude experimental

```bash
ctgbot component register claude/claude --runtime docker
ctgbot component claude/claude auth status

cat > .ctgbot/components/claude/claude/component.json <<'JSON'
{
  "model": "sonnet",
  "permission_mode": "bypassPermissions",
  "session_timeout_sec": 1800
}
JSON
```

### Gmail optional

```bash
ctgbot component register gmail/personal --runtime local

install -d -m 700 .ctgbot/components/gmail/personal
cat > .ctgbot/components/gmail/personal/component.json <<'JSON'
{
  "mailbox_email": "you@example.com"
}
JSON

cp oauth_client.json .ctgbot/components/gmail/personal/oauth_client.json
chmod 600 .ctgbot/components/gmail/personal/oauth_client.json

ctgbot component gmail/personal auth
ctgbot component gmail/personal auth status
```

### Inbound filters optional

Use one or more filters on a specific chat/source binding.

```bash
# Durable sender allowlist filter.
ctgbot component register filters/allowlist --runtime local

# LLM guard filter using a completion provider.
ctgbot component register llamacpp/qwen3-q5 --runtime backend
ctgbot component register guard/qwen --runtime local

install -d -m 700 .ctgbot/components/guard/qwen
cat > .ctgbot/components/guard/qwen/component.json <<'JSON'
{
  "completion": "llamacpp/qwen3-q5",
  "max_output_tokens": 512,
  "high_risk_score": 0.70
}
JSON
```

## Build runtime images

Runtime image targets are component-driven. Register components first, then build
images.

```bash
ctgbot image list
ctgbot image build --no-cache
```

## Run

```bash
ctgbot run
```

Use your preferred process supervisor for a real deployment.

## Enable a Telegram chat

Unknown channels are dropped until explicitly bound.

```bash
# 1. Start ctgbot.
ctgbot run

# 2. Send any message to the Telegram bot from the target chat.

# 3. In another shell, inspect dropped inbound channels.
ctgbot chat dropped

# 4. Bind the Telegram source and relay to a new ctgbot chat.
ctgbot chat bind telegram/telegram <external_channel_id> "My Chat"

# 5. List chats and pick the new chat id or short id.
ctgbot chat list

# 6. Bind components to the chat.
ctgbot chat <chat> component add agent codex/codex
ctgbot chat <chat> component add command process/process

# Optional trusted SQL access.
ctgbot chat <chat> component add command sql/sql

# Optional workspace.
ctgbot chat <chat> workspace set default

# Verify bindings.
ctgbot chat <chat> component list
```

## Add Gmail to a chat

A Gmail mailbox is an inbound source. The chat must also have a relay binding so
Gmail work is visible to an operator.

```bash
ctgbot chat <chat> component add source gmail/personal --external-channel-id you@example.com
ctgbot chat <chat> component list

# Optional: require known senders before Gmail reaches the agent.
ctgbot chat <chat> component gmail/personal filter add filters/allowlist

# Optional: classify inbound Gmail with the LLM guard.
ctgbot chat <chat> component gmail/personal filter add guard/qwen

# Show configured filters.
ctgbot chat <chat> component gmail/personal filter list
```

Allowlist commands are available in the bound chat after the allowlist filter is
configured:

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
/codex chat purge
/claude status
/claude container refresh
/thread list
/thread component bind gmail/personal
/thread component bind gmail/personal you@example.com
/thread <threadID> message send <message>
```

## Useful hostbridge commands

Agents can discover their available hostbridge surface from inside the runtime:

```bash
hostbridge help
hostbridge component list
hostbridge component <component> help
hostbridge thread list
hostbridge sendstdin
hostbridge sendfile <path>

# Gmail send/reply example from an agent runtime.
printf 'Hi there!' | hostbridge component gmail/personal messages send \
  --to sender@example.com \
  --subject 'Re: Subject' \
  --thread-id '<gmail_thread_id>' \
  --in-reply-to '<rfc_message_id>'
```

## Upgrade

```bash
ctgbot upgrade
ctgbot quit
# or from an enabled operator chat:
# /quit
```

## Notes

- Component secrets live in `.ctgbot/components/<type>/<name>/`.
- Telegram token lives in `.ctgbot/components/telegram/telegram/token.txt`.
- Codex auth lives in `.ctgbot/components/codex/codex/auth.json`.
- Bind inbound filters to a chat/source binding, not globally.
- Runtime image freshness notices tell agents when an operator should run
  `/upgrade` or refresh a component container.
