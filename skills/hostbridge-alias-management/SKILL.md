---
name: hostbridge-alias-management
description: Configure or review ctgbot Hostbridge aliases and exact workspace permissions when an operator asks to expose a host command or repository.
---

# Hostbridge alias management

Treat an alias as host-execution authority. Change it only with explicit operator
authorization. A skill explains the procedure; it does not grant permission.

## Preflight

1. Run `hostbridge status` and confirm the chat has the `ops command` component.
2. Inspect the effective value and its source with `hostbridge ops config get <key>`.
3. Verify executable and repository paths without reading credentials or invoking
   provider operations.

If `hostbridge ops` is unavailable, stop and ask the operator to enable it. The
host-side setup is normally:

```text
ctgbot component register ops/ops --runtime local
ctgbot chat <chat> component add command ops/ops
```

The chat must also use the workspace whose aliases are being changed. Binding
the `ops` component gives agents its operator commands; it does not authorize
regular chat users or create Hostbridge aliases by itself.

## Change an alias

- Prefer an existing purpose-specific `config.d` layer. List layers with
  `hostbridge ops config layers`.
- Use literal map-key syntax for alias names, especially names containing a
  hyphen: `allowed_commands["azure-devops"]`.
- Configure exact canonical host paths. For `allowed_cwds`, write the complete
  sorted, unique JSON array and preserve existing entries.
- Agents may invoke `/workspace/...` or workspace-relative paths; ctgbot maps
  them to the host workspace before enforcing the exact `allowed_cwds` list.
- Never replace exact roots with a parent directory. Do not add a shell,
  unrestricted arguments, credentials, environment variables, stdin, or Git
  refspec authority unless each expansion was explicitly approved.
- Keep typed Git subcommands closed. Treat direct typed clients separately from
  installing their binaries.

Example exact-cwd update:

```text
hostbridge ops config set 20-git-aliases 'workspaces.main.hostbridge.allowed_commands["git"].allowed_cwds' '["/absolute/repository-a","/absolute/repository-b"]'
```

Read the effective value back after writing it. Alias snapshots are loaded for
the lifetime of the ctgbot host process, so the operator must restart ctgbot.
An image rebuild or container refresh is not required solely for an alias
change. After restart, verify discovery and a harmless read-only invocation
from an agent runtime.
