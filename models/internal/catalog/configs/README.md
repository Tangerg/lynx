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
      "knowledge_cutoff": "2024-07-01T00:00:00Z",
      "pricing": [
        { "input_per_1m": 0.55, "output_per_1m": 2.19, "cache_read_per_1m": 0.14 }
      ],
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
- `pricing` is an **array of rate bands** (USD per 1,000,000 tokens),
  ascending by `threshold`. Usually one band (threshold omitted = 0, the
  base). Omit `pricing` for a metadata-only row; omit `cache_*` within a
  band when unknown (`CostOf` falls back to that band's input rate).
- **Tiered pricing** — long-context models (Gemini 2.5 Pro, some OpenAI)
  reprice above a token threshold, expressed as extra bands.
  **A band reprices the *whole* prompt, not the marginal tokens** — a
  250K-token Gemini 2.5 Pro call bills entirely at the >200K band.
  `CostOf` selects the highest band the prompt's input size reaches
  (scanning back to front):

  ```json
  "pricing": [
    { "input_per_1m": 1.25, "output_per_1m": 10, "cache_read_per_1m": 0.125 },
    { "threshold": 200000,
      "input_per_1m": 2.5, "output_per_1m": 15, "cache_read_per_1m": 0.25 }
  ]
  ```
- `reasoning.supported` is the authoritative "can reason" bit; `levels` /
  `default_level` apply only when effort is level-controlled (OpenAI,
  Gemini, …). A token-budget reasoner is just `{"supported": true}`.
- `modalities.input` / `modalities.output` list the media the model takes
  and emits (`text`, `image`, `audio`, `video`, `pdf`). Output is
  `["text"]` for chat models.
- `tool_call` / `structured_output` flag tool/function calling and a
  native structured-output feature.
- `knowledge_cutoff`, `release_date`, `last_updated` are `time.Time`
  (RFC3339 in JSON); month-only sources land on the first of the month.
  `limits` (`context_window`, `max_input_tokens`, `max_output_tokens`)
  and the dates are optional — omit when unknown.
- `deprecated` flags a retired model. Such models are **kept** in the
  catalog (cost still attributes for callers on the old id); consumers
  hide or flag them via this bool. Omit (false) for current models.

Only chat models are included — embedding, TTS, and image-generation
models (output modality not `text`, or an embedding `family`) are filtered
out during generation.

## Source / maintenance

Rows are generated from **[models.dev](https://models.dev)** — a community
model database (also used by LangChain's model profiles) whose data lives
as per-model TOML. Mapping:

- `name` → `display_name`; `knowledge` → `knowledge_cutoff`;
  `release_date` / `last_updated` → same names; `status == "deprecated"`
  → `deprecated: true` (beta/alpha statuses are ignored — they're current).
- `[cost]` `input` / `output` / `cache_read` / `cache_write` → the base
  `pricing` band. `[[cost.tiers]]` with `tier.type == "context"` → extra
  bands (`threshold` = `tier.size`); other tier types are skipped.
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
