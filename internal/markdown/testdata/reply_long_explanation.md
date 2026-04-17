### Plan

We should keep the happy path simple.
We should also make sure chunking stays deterministic.

The parser should preserve lines.
The renderer should preserve intent.
The provider should own fallback.

```text
chunk 1
chunk 2
chunk 3
```

After that, we can tighten edge cases with focused tests rather than more architecture churn.
