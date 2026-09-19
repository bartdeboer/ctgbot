# GitHub Copilot agent (initial integration)

Docker-only agent provider beside Codex/Claude; not a GitHub API or inference
backend. The CLI contract and native image recipe pin **1.0.83**. Fake tests do
not certify employer authentication or live image compatibility.

## Operator setup (separate approval required)

1. Confirm the account has a Copilot seat, effective organization/enterprise
   **Copilot CLI** access, an approved model and permission to send the workspace
   to Copilot. Respect SSO, managed settings and employer network/sandbox policy.
2. Register a separate profile; do not repoint an existing Codex/Claude binding:
   `ctgbot component register copilot/copilot --runtime docker`.
3. Regenerate embedded build assets through the normal source release workflow,
   then use `ctgbot image build` in the instance directory. Dependency chain:
   go-node-python-base → copilot-base → copilot. Do not install the CLI on the host.
4. Run `ctgbot component copilot/copilot auth` explicitly from an operator terminal.
   This uses the device-code flow, not inference. Headless Linux may ask consent
   to store OAuth in plaintext; do not automate that consent. The component's
   `.copilot/config.json` is sensitive. Never copy unrelated GitHub credentials.
5. Once image/auth/model acceptance is authorized and complete, attach with
   `ctgbot chat <chat> component add agent copilot/copilot`.

`auth status` intentionally reports **unverified**, without reading credentials,
starting containers or contacting GitHub. It is not an authentication health test.
OAuth device-code login is the primary supported path. Ordinary
`GH_TOKEN`/`GITHUB_TOKEN` are removed
from the launched agent environment. Do not configure `COPILOT_GITHUB_TOKEN` for
this OAuth setup: the CLI can prioritize it over saved OAuth.

Copilot may fall back to `gh` credentials available **inside the sandbox** if its
own credentials are unavailable. This accepted limitation matters if users install
or configure `gh` in a custom image or durable agent home. Default recipes do not
intentionally install `gh` or mount host credentials/keychains; no such mounts are
authorized by this integration. This is not evidence of host credential exposure.

### OAuth persistence and refresh

Auth and turns mount the same component profile at `/profile/components/copilot/<name>` and
set `COPILOT_HOME` to its `.copilot` directory. Auth uses the profile as HOME;
turns keep `/home/agent`. The explicit Copilot home, not the thread HOME, selects
Copilot configuration and session storage. Do not override `COPILOT_HOME` in
runtime/factory environment settings or repoint the profile during refresh.

For the headless, operator-consented file-storage path, preserve the complete
component profile, especially sensitive `.copilot/config.json` and session state.
Do not paste credentials into chat, logs or tests. Container refresh must reuse
this profile and the existing thread mapping; session resume is not a login.
If the CLI selects an OS keychain instead, the profile bind alone does **not**
prove that credentials persist or remain accessible in another container. Stop
for persistence review rather than extracting keys; fallback is not proof that
the component OAuth store survived refresh.

Fake composition tests confirm the same mount/config paths and synthetic stored
OAuth/session reuse after runtime reconstruction. They do not certify native
OAuth or actual container refresh. After
separate approval, operator acceptance must verify the intended account, a turn,
refresh and explicit session resume without a second login. `auth status` remains
unverified; absence of a login prompt is not proof of the correct account.

Optional component-home `component.json`:

```json
{"model":"<approved-model-id>","session_timeout_sec":1800}
```

Unset model leaves selection to the CLI, including a resumed session’s saved model. Existing `runtime.json` handles image,
context/dependencies, UID/GID and environment. Per-thread commands include
`copilot config set model <id>`, `copilot config unset model`, lifecycle/status,
and `copilot config set container.keep-running true`. Default is stop after turn.
No goal/compact command, ACP/SDK daemon, cloud delegation or native-host runtime.

## Behavior and limits

- Each thread/component mapping holds an explicit UUID; refresh retains it,
  purge selects a new conversation. No latest-session continuation. Invalid,
  missing or mismatched output cannot replace a mapping. A well-formed failed
  terminal result can deliberately retain its validated expected UUID for retry.
- Prompts/bootstrap are private staged files, not large argv. Only the last
  complete main-agent answer becomes broker Final; no reasoning/tool/subagent
  transcript is relayed. Artifacts use existing Hostbridge sendfile.
- CLI auto-update and **built-in MCP** are disabled. This is **not all MCP off**:
  trusted workspaces, plugins and user/managed configuration can still load MCP.
  Only local read/write/shell tool permissions are granted explicitly. No policy
  files are rewritten, blanket approval added or sandbox bypass requested.
- Prompt-mode memory is not enabled. This does not disable persisted sessions,
  logs or cross-session storage in the component's shared `COPILOT_HOME`.
  Remote export/control are disabled; this is not a promise of zero telemetry.
- Cancellation inherits the existing direct-process runtime: synchronous Exec
  joins output, Interrupt requests SIGINT, default cleanup requests Docker stop.
  Killing the Docker client is not proof of in-container or descendant exit.
  Lock waits and best-effort stop have inherited limits; `keep_running` is not
  silently overridden. No new termination guarantee beyond existing providers.
- If employer policy enforces Copilot's nested sandbox, its Linux dependencies
  and Hostbridge connectivity need separate acceptance—not extra host privileges.

Sources: [pinned release](https://github.com/github/copilot-cli/releases/tag/v1.0.83),
[authentication](https://docs.github.com/en/copilot/how-tos/copilot-cli/set-up-copilot-cli/authenticate-copilot-cli).

## Last response usage

`copilot thread info` (also available for `codex` and `claude`, including via
Hostbridge) reads the latest finalized response for the current thread/component
and provider session from the database. It works after restarts and days later
without starting a container or contacting a provider. Older responses created
before this feature have no usage; unavailable fields are not reported as zero.

Copilot writes usage into the existing unique temporary prompt directory. ctgbot
reads its bounded snapshot before cleanup and stores only selected typed counts
with the final message. Its scope is the provider's end-of-invocation snapshot,
not a computed last-turn delta. Codex reports turn counts; Claude uses invocation
model totals (including subagents) where supplied, otherwise explicitly labelled
main-agent usage. Input includes cache reads/writes; cache-read share is shown only
when both counts are known. Costs and account balances are not inferred.

This is final-response diagnostics, not complete billing history: failures without
a final response produce no usage row. A latest final with missing metrics shows
unavailable, not older counts. Conversation reset/session mismatch hides old usage;
message-history purge removes the corresponding fields with the messages.

Use `/thread info` to show the same persisted usage grouped by enabled agent (including named components); provider-prefixed commands remain available. This is a DB-only query, not a live usage or balance request.
