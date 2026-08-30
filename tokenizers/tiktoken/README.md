# tiktoken

`tiktoken` implements the `core/tokenizer` capabilities with OpenAI's tiktoken
vocabularies, adapting `github.com/pkoukk/tiktoken-go` to the small interfaces
Core defines.

## Install

```bash
go get github.com/Tangerg/scope/tokenizers/tiktoken
```

## Usage

The vocabulary is chosen explicitly. There is no default, because no single
encoding is correct across models:

```go
tokenizer, err := tiktoken.New(tiktoken.O200KBase)
if err != nil {
    return err
}

count, err := tokenizer.CountText(ctx, "hello")
```

An unknown encoding returns `ErrInvalidEncoding` at construction rather than
silently falling back to another vocabulary.

## What this module does not own

No provider request, no model-to-encoding routing, no text splitting, and no
application cache. Those belong to the caller — splitting to `etl`, routing to
whatever knows the model.

See [`ARCHITECTURE.md`](ARCHITECTURE.md) for the boundaries this rests on.
