---
name: custom-agent-image
description: Add toolchains such as Java and Maven to ctgbot agent images while preserving the normal image build, component configuration and container refresh workflow.
---

# Custom agent image

Use the existing component image build chain, not a parallel manual installation
inside disposable containers. This skill is repository guidance; reading it does
not install a skill, build images or authorize a live rollout.

## Establish the target

- Confirm the ctgbot checkout, instance directory, agent type and exact component
  registration/profile. Preserve unrelated source and runtime configuration.
- Select required toolchain versions and host architecture with the operator.
- Image selection is component-scoped (`runtime.json`), not currently per-thread.
  Changing a shared registration affects every chat/thread using it.
- Prefer a distinct derived image for optional tools; do not overwrite standard
  ctgbot image tags. Keep Hostbridge, agent binaries and supervisor tooling intact.
  Never bake credentials, Maven settings containing passwords, or private state in.

## Recipe and dependency chain

For example, add `docker/codex-work.Dockerfile` deriving from
`ctgbot-codex:latest`, installing the required JDK/Maven. Check the actual parent
OS, user and package availability first. Preserve inherited runtime behavior;
keep dependency caches/project output outside permanent toolchain layers where
practical. A layer above Codex is simplest, but its packages may rebuild whenever
the parent changes. Only move it lower if that cost justifies more recipe wiring.

The builder consumes an embedded tar context. `internal/buildassets/files.go`
maps `docker/` to the context root: configure `codex-work.Dockerfile`, **not**
`docker/codex-work.Dockerfile` or an arbitrary file in the instance directory.
Additional COPY inputs must also be included in the selected context. Installed
binaries use their embedded context; source-mode fallback is not a way to update
an already installed binary's recipes.

Merge the following image fields into the selected component's `runtime.json`,
retaining its existing env/user/GPU/other settings. This is an example after adding
the work Dockerfile, not a recipe already shipped by ctgbot:

```json
{
  "image": "ctgbot-codex-work:latest",
  "dockerfile": "codex-work.Dockerfile",
  "uses": {
    "Name": "codex-standard",
    "Image": "ctgbot-codex:latest",
    "Dockerfile": "codex.Dockerfile",
    "NoCache": true,
    "Uses": {
      "Name": "codex-base",
      "Image": "ctgbot-codex-base:latest",
      "Dockerfile": "codex.base.Dockerfile",
      "Uses": {
        "Name": "go-node-python-base",
        "Image": "ctgbot-go-node-python-base:latest",
        "Dockerfile": "go-node-python.base.Dockerfile"
      }
    }
  }
}
```

Nested targets above use the current `runtimeimage.Target` JSON field names
(in particular `NoCache`, not `no_cache`). `uses` schedules dependencies; it does not rewrite Dockerfile FROM instructions.
Declare the entire needed chain: a nested target does not automatically acquire
the component's default dependencies. Every FROM tag must match the corresponding
built image. Adapt to Claude/other agents using their actual recipes, not Codex's.

## Validate and roll out only within the authorized scope

1. Check embedded-file inclusion, runtime config parsing, dependency order and
   image/tag agreement. Relevant code/tests: `internal/buildassets`,
   `internal/runtime/image`, `internal/app/runtime_images.go`, and the selected
   agent's `RuntimeImageTargets` implementation/tests.
2. From the source root, the normal `go run ./cmd/ctgbot install` path regenerates
   and embeds build assets before installing. `go run ./cmd/pack` is the explicit
   generation entry point when needed; generation alone does not replace the
   installed binary. Inspect generated changes; do not commit build artifacts
   contrary to repository conventions.
3. From the **instance directory**, run `ctgbot image list`, then
   `ctgbot image build`. Check the complete dependency chain before building.
   `--no-cache` is for a deliberate full rebuild, not the default for every edit.
4. Coordinate a host restart for changed resident component configuration and
   refresh only the affected containers. Discover current scoped refresh commands
   through help; a rebuild alone does not replace an existing container.
5. Verify selected image identity, Java/Maven/tool versions, Hostbridge and one
   representative project build. Keep source tests distinct from real Docker and
   live agent validation. Retain the previous profile/image for rollback.

Host builds/configuration require the operator's existing authorized host route;
container-local Docker or a repository skill does not grant host access. If that
route is unavailable, provide the operator commands rather than widening aliases.
