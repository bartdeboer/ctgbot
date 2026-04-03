#!/bin/sh
set -eu

CODEX_HOME="${CODEX_HOME:-/codex-home}"
CODEX_SHARED_HOME="${CODEX_SHARED_HOME:-/codex-shared}"

mkdir -p "$CODEX_HOME"
mkdir -p "$CODEX_SHARED_HOME"

for file in auth.json config.toml models_cache.json .codex-global-state.json; do
    ln -sf "$CODEX_SHARED_HOME/$file" "$CODEX_HOME/$file"
done

for dir in cache memories skills vendor_imports; do
    mkdir -p "$CODEX_SHARED_HOME/$dir"
    ln -snf "$CODEX_SHARED_HOME/$dir" "$CODEX_HOME/$dir"
done

exec "$@"
