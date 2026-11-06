# Codex browser runtime libraries

Both existing Codex-specific bases install the same 19 requested runtime-library
packages, using apt dependency resolution and --no-install-recommends:

- standard: go-node-python.base.Dockerfile (Debian Bookworm) →
  codex.base.Dockerfile → codex.Dockerfile;
- GPU: go-node-python-cuda.base.Dockerfile (Ubuntu 22.04 CUDA) →
  cuda.base.Dockerfile → cuda.Dockerfile.

There is no common Codex-only parent shared by these distro-specific chains.
The short apt block is therefore repeated in the two Codex bases, before the
existing Codex installer, rather than expanding the generic development bases
used by unrelated agents or adding a new image framework. Keep these blocks in
sync; the build-context regression test verifies both packaged copies.

Package names match the director's successful Ubuntu dependency installation.
Bookworm also uses libasound2/libatk-bridge2.0-0/libgbm1 (not newer-distro t64
renames); package versions and transitive dependencies are distro-specific.
Sources: https://packages.debian.org/bookworm/libasound2 ,
https://packages.debian.org/bookworm/libatk-bridge2.0-0 ,
https://packages.debian.org/bookworm/libgbm1 .
No Ubuntu version pins are applied to Debian. As with existing apt recipes,
repositories resolve versions at build time; this is not a snapshot-pinned build.

Apt installs required transitive font support; no extra font collections are
requested. The observed Ubuntu transaction added 52 packages (19 requested),
reported 27.5 MB installed and fetched 8.1 MB. This is not a promised Debian/GPU
image-layer delta: parent contents, distro versions and compression differ.
Apt caches and lists are cleaned in the same RUN layer.

No Chrome/Chromium executable, Playwright/Node package, driver, service, browser
flag, capability or runtime policy is added. Chrome remains separately cached
under persistent /home/agent. GPU support, agent bindings, provider profiles and
session mappings are unchanged. Per-thread seccomp remains a separate explicit
operator opt-in; adding libraries alone does not enable the browser sandbox.

Existing RuntimeImageTargets/Uses chains discover these same recipes with no
graph/config change. internal/buildassets/files.go already includes docker/ at
the embedded context root. An installed ctgbot binary must be upgraded with
regenerated build assets through the existing install/upgrade path before its
image builds can consume this recipe change. Rebuild the relevant chain, then
refresh selected containers separately; neither has been executed for this
source checkpoint. Existing custom external contexts are not implicitly changed.

Source validation: packaged-context regression checks both parents, exact
library block parity, no-recommends, same-layer cleanup and no browser install;
existing component image-chain and build-planner tests remain applicable.
No image build/pull, apt operation or fresh-image browser validation is claimed.
