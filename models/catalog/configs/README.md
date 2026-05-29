# catalog configs

One JSON per provider — the maintained per-model metadata table behind
`catalog.Lookup` / `catalog.Models`. Embedded via `go:embed configs/*.json`;
**adding a provider is just dropping a `<provider>.json` here** (no code
change).

## Schema

Each model entry is a `chat.ModelInfo` (pricing is nested):

```json
{
  "provider": "deepseek",
  "models": [
    { "id": "deepseek-v4-flash",
      "display_name": "DeepSeek V4 Flash",
      "pricing": {
        "input_per_1m": 0.14, "output_per_1m": 0.28,
        "cache_read_per_1m": 0.0028
      },
      "reasoning": {
        "supported": true,
        "levels": ["high", "xhigh"],
        "default_level": "high"
      },
      "modalities": {
        "input": ["text", "image", "pdf"],
        "output": ["text"]
      },
      "context_window": 1000000,
      "max_tokens": 384000 }
  ]
}
```

- `provider` must equal the adapter's `Provider` const, lowercased
  (`Lookup` matches case-insensitively): `"Anthropic"` → `"anthropic"`,
  `"Google"` → `"google"`, `"DeepSeek"` → `"deepseek"`. OpenAI-compat
  providers delegate to `openai.NewChatModel` with their own `Provider`,
  and the lookup keys off that — so their config is keyed by their name
  (`deepseek`, `groq`, `xai`, …), not `openai`. Likewise `vertexai`
  delegates to `google.NewChatModel` but keeps Provider `"VertexAI"`.
- `pricing` rates are USD per 1,000,000 tokens; omit `cache_*` when
  unknown (`Cost` falls back to the input rate). Omit `pricing` entirely
  for a metadata-only row (capabilities still useful) — a zero `Pricing`
  means "cost unknown", not "free".
- `reasoning.supported` is the authoritative "can reason" bit; `levels` /
  `default_level` apply only when effort is level-controlled (OpenAI,
  Gemini). A token-budget reasoner (Anthropic) is just
  `{"supported": true}`. Omit `reasoning` for non-reasoning models.
- `modalities.input` / `modalities.output` list the media types the model
  takes and emits (`text`, `image`, `audio`, `video`, `pdf`), text first.
  Output is `["text"]` for chat models. Tool calling and structured
  output are *deliberately not modeled*: there's no maintained per-model
  source for them and they're near-universal among the chat models lynx
  adapts, so a false default would misreport "no data" as "unsupported".
- `context_window` / `max_tokens` are optional; omit when unknown.

## Source / maintenance

Most rows are mirrored from **charm.land/catwalk** (a community model
catalog) with this mapping:

- `cost_per_1m_in` → `pricing.input_per_1m`, `cost_per_1m_out` →
  `pricing.output_per_1m` (exact).
- `name` → `display_name`; `context_window` / `default_max_tokens` →
  `context_window` / `max_tokens`.
- `can_reason` / `reasoning_levels` / `default_reasoning_effort` →
  `reasoning.supported` / `reasoning.levels` / `reasoning.default_level`.
- `supports_attachments` gates `modalities`: an attachment-capable model
  gets its vendor's full input profile (Anthropic `text/image/pdf`, OpenAI
  `text/image/pdf`, Google/Vertex `text/image/audio/video/pdf`, xAI/Zhipu
  `text/image`), grounded in each vendor's own model card / capabilities
  API; otherwise input is `text`-only. catwalk has just one attachment
  bool, so the per-modality breakdown is this profile, not raw catwalk
  data — refresh the profiles when a vendor adds an input type.
- catwalk's two cached fields (`cost_per_1m_in_cached`,
  `cost_per_1m_out_cached`) are used inconsistently across providers
  (Anthropic puts the write premium in `in_cached` and the read discount
  in `out_cached`; DeepSeek/OpenAI put the read discount in `in_cached`),
  so each nonzero cached rate is classified by comparison to input:
  **below input = a read discount** (`cache_read_per_1m`), **above input
  = a write premium** (`cache_write_per_1m`). This is provider-agnostic
  and correct regardless of which catwalk field the rate sat in.

To refresh or extend, re-run the catwalk transform or edit a JSON
directly. Only providers lynx has a chat adapter for are included.
`anthropic.json` additionally carries `claude-3-5-haiku-20241022` (lyra's
default model, dropped from catwalk's current list) as a hand-curated row.
