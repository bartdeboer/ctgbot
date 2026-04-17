## Quick take

This looks **much cleaner now**.

The big win is that chunking is based on the document structure instead of trying to guess transport formatting too early. That makes the whole flow feel more *predictable*.

A few things I especially like:

- semantic chunking first
- provider-owned fallback
- a simpler rendering surface
- realistic fixture tests

Here's a tiny example:

```go
func sendChunk(doc *Document) error {
    // render preferred format
    // fallback if Telegram rejects it
    return nil
}
```

If I were summarizing the current state in one sentence:

**The architecture is now simple enough to reason about, but still flexible enough to degrade gracefully when Telegram gets picky.**

And yes — `Chunked()`, `Render()`, and fallback-at-send-time feels like the right split.
