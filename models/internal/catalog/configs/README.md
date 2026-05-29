# catalog configs

One JSON per provider — the maintained per-model metadata table behind
`catalog.Lookup` / `catalog.Models`. Embedded via `go:embed configs/*.json`;
**adding a provider is just dropping a `<provider>.json` here** (no code
change). Generated from [models.dev](https://models.dev) — see *Source*.

## Schema

Each model entry is a `chat.ModelInfo`:

```json
{
  "provider": "deepseek",
  "models": [
    { "id": "deepseek-reasoner",
      "display_name": "DeepSeek Reasoner",
      "knowledge_cutoff": "2024-07",
      "pricing": {
        "input_per_1m": 0.55, "output_per_1m": 2.19,
        "cache_read_per_1m": 0.14
      },
      "reasoning": {
        "supported": true,
        "levels": ["high", "xhigh"],
        "default_level": "high"
      },
      "modalities": {
        "input": ["text"],
        "output": ["text"]
      },
      "tool_call": true,
      "structured_output": true,
      "limits": {
        "context_window": 128000,
        "max_output_tokens": 65536
      } }
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
  for a metadata-only row — a zero `Pricing` means "cost unknown", not
  "free".
- `reasoning.supported` is the authoritative "can reason" bit; `levels` /
  `default_level` apply only when effort is level-controlled (OpenAI,
  Gemini, …). A token-budget reasoner is just `{"supported": true}`.
- `modalities.input` / `modalities.output` list the media the model takes
  and emits (`text`, `image`, `audio`, `video`, `pdf`). Output is
  `["text"]` for chat models.
- `tool_call` / `structured_output` flag tool/function calling and a
  native structured-output feature.
- `knowledge_cutoff` and `limits` (`context_window`, `max_input_tokens`,
  `max_output_tokens`) are optional; omit when unknown.

Only chat models are included — embedding, TTS, and image-generation
models (output modality not `text`, or an embedding `family`) are filtered
out during generation.

## Source / maintenance

Rows are generated from **[models.dev](https://models.dev)** — a community
model database (also used by LangChain's model profiles) whose data lives
as per-model TOML. Mapping:

- `name` → `display_name`; `knowledge` → `knowledge_cutoff`.
- `[cost]` `input` / `output` / `cache_read` / `cache_write` →
  `pricing.*_per_1m`.
- `reasoning` (bool) → `reasoning.supported`. models.dev has no effort
  levels, so `reasoning.levels` / `default_level` are **backfilled from
  charm.land/catwalk** (`reasoning_levels` / `default_reasoning_effort`)
  where it has the model. To be revisited with per-vendor data.
- `[modalities]` `input` / `output` → `modalities.*` (real per-model
  arrays, not a heuristic).
- `tool_call` / `structured_output` → same names.
- `[limit]` `context` / `input` / `output` → `limits.context_window` /
  `limits.max_input_tokens` / `limits.max_output_tokens`.
- `[extends]` is resolved (a wrapper model — e.g. most `google-vertex`
  entries — overlays the canonical model it points at). This is why
  `vertexai` mirrors `google`'s Gemini lineup.

To refresh or extend, re-run the models.dev transform over the providers
lynx has a chat adapter for. Local augmentations (e.g. reasoning levels a
vendor publishes but models.dev lacks) can be merged in the same step,
mirroring LangChain's `profile_augmentations` approach.
