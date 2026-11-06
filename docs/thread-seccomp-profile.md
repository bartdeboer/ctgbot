# Opt-in thread seccomp profile

Docker thread runtimes accept an optional persistent thread setting:
`runtime.seccomp_profile`. Unset retains the component's existing seccomp mode;
set replaces it for that thread. Null-thread/auth containers retain the component
mode. This affects **all processes and Docker agent runtimes using the thread**,
not just Chrome. Other runtime backends are unchanged.

Only Root/operator actors may set **or unset** this field. Enforcement is in the
messaging service mutation boundary, not just command help. Ordinary thread
configuration permissions are unchanged. The operator uses the normal thread
config command through a Root-authorized control surface (an ordinary agent
Hostbridge request is intentionally denied):

```text
thread <id> config set runtime.seccomp_profile /operator/policies/Browser-v1.json
thread <id> config unset runtime.seccomp_profile
```

The path is an absolute path on the **ctgbot/Docker CLI host**, with case preserved.
The Docker CLI reads the file; it is not a container mount or daemon-VM path.
Provision a reviewed versioned/checksummed complete profile in operator-controlled
storage **outside all agent-writable workspace, HOME and component-profile
mounts**. Keep its parent directories operator-controlled too. Do not use a
three-rule excerpt in place of the full Docker-default-derived profile.

Set and runtime preparation check readability, a regular non-symlink file,
1 MiB bound, JSON object/defaultAction and syscall-list shape. Docker remains the
final policy interpreter. These checks are not a policy audit, filesystem
authorization mechanism, or a guarantee against operator replacement between
validation and Docker reading. Host storage integrity is an operator prerequisite.
There is no fallback on invalid/missing files and no profile installation here.

Upgrade/restart ctgbot to load the new implementation, then have the operator
assign the setting and **refresh only the selected container**. Existing running
or stopped containers are reused; stop/start does not apply a new profile.
Rollback: unset, then refresh the selected container. No automatic recreation,
global defaults, image changes or Chrome flags are introduced.

Live acceptance is separate: compare the same pinned image and non-root user
with default and candidate policy in disposable bounded containers, without
credentials, real workspace, network or ports. Verify namespace creation and
Chrome renderer sandbox evidence, not merely exit 0. If inherited filters or
host restrictions still deny operations, stop and diagnose; do not substitute
unconfined, SYS_ADMIN, privileged or no-sandbox. Browser dependencies and image
size are separate concerns. This source change does not establish browser
compatibility or authorize live policy assignment.
