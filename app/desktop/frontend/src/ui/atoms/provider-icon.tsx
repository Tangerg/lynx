// Brand icon for an LLM provider, from @lobehub/icons. Maps a provider id
// (Provider.type / Model.provider, e.g. "deepseek") to its brand mark; falls
// back to a neutral spark glyph for providers we don't have a brand for.
//
// We import each brand's Mono component by deep path
// (`es/<Brand>/components/Mono`) rather than the package barrel: the barrel
// (and each brand's index) pulls in the `.Avatar` variant, which depends on
// `@lobehub/ui` (not installed) and breaks the build. Mono is monochrome
// (currentColor), which also keeps the picker visually consistent.

import type { ComponentType } from "react";
import Anthropic from "@lobehub/icons/es/Anthropic/components/Mono";
import DeepSeek from "@lobehub/icons/es/DeepSeek/components/Mono";
import Gemini from "@lobehub/icons/es/Gemini/components/Mono";
import Meta from "@lobehub/icons/es/Meta/components/Mono";
import Mistral from "@lobehub/icons/es/Mistral/components/Mono";
import Moonshot from "@lobehub/icons/es/Moonshot/components/Mono";
import Ollama from "@lobehub/icons/es/Ollama/components/Mono";
import OpenAI from "@lobehub/icons/es/OpenAI/components/Mono";
import Qwen from "@lobehub/icons/es/Qwen/components/Mono";
import Zhipu from "@lobehub/icons/es/Zhipu/components/Mono";
import type { IconSize } from "@/lib/iconScale";
import { Icon } from "@/ui/icons";

type BrandIcon = ComponentType<{ size?: number }>;

// Keyed by lowercased provider id/type. Aliases map vendor synonyms onto the
// same brand mark (e.g. kimi → Moonshot, claude → Anthropic).
const BRAND: Record<string, BrandIcon> = {
  deepseek: DeepSeek,
  openai: OpenAI,
  anthropic: Anthropic,
  claude: Anthropic,
  gemini: Gemini,
  google: Gemini,
  meta: Meta,
  llama: Meta,
  mistral: Mistral,
  moonshot: Moonshot,
  kimi: Moonshot,
  ollama: Ollama,
  qwen: Qwen,
  zhipu: Zhipu,
};

export function ProviderIcon({ provider, size = "md" }: { provider: string; size?: IconSize }) {
  const Brand = BRAND[provider.toLowerCase()];
  // A brand mark is a filled logo, not a stroked glyph, so it takes the ladder's
  // box but none of its stroke. The box comes from the custom property rather than
  // a number because these sit inline with labels that follow the user's base size.
  if (Brand) {
    return (
      <span
        aria-hidden
        className="inline-grid shrink-0 place-items-center [&>svg]:size-full"
        style={{ width: `var(--icon-${size})`, height: `var(--icon-${size})` }}
      >
        <Brand size={0} />
      </span>
    );
  }
  return <Icon name="spark" size={size} />;
}
