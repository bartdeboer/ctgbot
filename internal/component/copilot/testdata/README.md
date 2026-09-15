# Copilot CLI 1.0.83 synthetic fixtures

These are **synthetic**, public-contract-derived fixtures, not recordings of an
employer account or successful live inference. No secrets or employer code.

Contract sources: the published `@github/copilot-linux-x64@1.0.83` package,
`app.js` JSONL emission and `schemas/session-events.schema.json`.

- app.js SHA256: 20230edbee06d6236148a049e84e4e4507100f51c7a4a9a0e5afe056c41c816b
- schema SHA256: bea5ebe5060d68b70ce366f46b866d8dd19c252ef1df6445905cd2377954458e
- Metadata: https://registry.npmjs.org/@github/copilot-linux-x64/1.0.83
- Release: https://github.com/github/copilot-cli/releases/tag/v1.0.83

`result` is a CLI wrapper record, not a session schema event; its sessionId and
exitCode are top-level and it contains no answer. The parser deliberately only
models the fields needed for identity, complete main-agent text and termination.
Runtime executable/image/auth compatibility requires separate acceptance.
