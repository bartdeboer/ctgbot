# Completion from stdin

Use a short quoted instruction plus explicit trailing `--stdin`:

```sh
cat document.txt | hostbridge llamacpp completion "Summarize this document" --stdin
cat document.txt | hostbridge llamacpp model qwen3.5-9b-q4 completion "Summarize this document" --stdin
cat document.txt | hostbridge llamacpp/mac-local model my-model completion "Summarize this document" --stdin
```

Named component prefixes must already be registered/available. No configuration,
backend or model defaults change. Prompt-only completion continues to work.

The client captures a pipe or redirected file only for the explicitly parsed
stdin completion command; it does not read an interactive terminal. The document
travels in Request.Stdin, never shell argv. Standalone ctgbot CLI/message callers
that request --stdin without supplying Request.Stdin receive an error, not an
implicit read or an empty inference.

The command owner enforces a **1 MiB (1,048,576 byte) combined instruction and
document budget** before acquiring a backend. This is a transport/input byte
budget, **not a model context guarantee**. Shorten/chunk text to fit the selected
model's token context; this command does not automatically chunk or summarize.

Instruction and document must be nonblank valid UTF-8. Document quotes,
backslashes, leading/trailing spaces and newlines are preserved. The instruction
becomes a system message and the document a separate user message using the
existing CompletionPrompt contract. This separation is not a prompt-injection
security guarantee.

Validation errors do not echo document contents. Backend error details are
withheld for this mode because providers can echo input in error bodies.
Generated output can naturally quote the document; this is not a no-retention
or secret-redaction feature.

The existing typed catalog, authorization, Gob transport, response handling and
backend session policy are reused. No streaming response or universal Gob
cancellation propagation guarantee is introduced.
