# pricing configs

One JSON per provider — the maintained per-model rate table behind
`pricing.Lookup`. Embedded via `go:embed configs/*.json`; **adding a
provider is just dropping a `<provider>.json` here** (no code change).

## Schema

```json
{
  "provider": "deepseek",
  "models": [
    { "id": "deepseek-v4-flash",
      "input_per_1m": 0.14, "output_per_1m": 0.28,
      "cache_read_per_1m": 0.0028, "cache_write_per_1m": 3.75 }
  ]
}
```

- `provider` must equal the adapter's `Provider` const, lowercased
  (`Lookup` matches case-insensitively): `"Anthropic"` → `"anthropic"`,
  `"Google"` → `"google"`, `"DeepSeek"` → `"deepseek"`. OpenAI-compat
  providers delegate to `openai.NewChatModel` with their own `Provider`,
  and the lookup keys off that — so their config is keyed by their name
  (`deepseek`, `groq`, `xai`, …), not `openai`.
- rates are USD per 1,000,000 tokens; omit `cache_*` when unknown
  (`Cost` falls back to the input rate).

## Source / maintenance

Most rows are mirrored from **charm.land/catwalk** (a community model
catalog) with this mapping:

- `cost_per_1m_in` → `input_per_1m`, `cost_per_1m_out` → `output_per_1m`
  (exact).
- catwalk's `*_cached` fields are inconsistent across providers, so each
  nonzero cached rate is classified by comparison to input: **below
  input = a read discount** (`cache_read_per_1m`), **above input = a
  write premium** (`cache_write_per_1m`). This is provider-agnostic and
  correct regardless of which catwalk field the rate sat in.

`anthropic.json` / `openai.json` are hand-curated (current models lyra
ships with). To refresh or extend, edit the JSON directly or re-run the
catwalk transform for the new rows. Only providers lynx has a chat
adapter for are included.
