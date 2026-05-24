// IconShowcase — compact, curated subset of @lobehub/icons rendered in
// the Settings pane. Sibling to IconGallery (the full browser); this one
// just shows the brands most users care about, grouped by purpose, so the
// pane fits the rail-style settings layout.

import { IconMap, TocById } from "./iconMap";

type Section = { title: string; ids: string[] };

// Hand-curated list. Keep each row to ~6–10 brands so the grid stays tidy
// at the settings pane width. Order within a section is intentional
// (recognition order, not alphabetical).
const SECTIONS: Section[] = [
  {
    title: "Frontier labs",
    ids: [
      "OpenAI", "Anthropic", "Claude", "ClaudeCode", "Gemini", "Google",
      "Grok", "Meta", "DeepSeek", "Mistral", "Cohere", "Perplexity",
    ],
  },
  {
    title: "Cloud & enterprise",
    ids: [
      "Microsoft", "Azure", "Bedrock", "Aws", "GoogleCloud",
      "Nvidia", "IBM", "Apple", "Github",
    ],
  },
  {
    title: "Chinese ecosystem",
    ids: [
      "Qwen", "Doubao", "Kimi", "Wenxin", "Hunyuan", "ChatGLM",
      "Yi", "Minimax", "Spark", "SenseNova",
    ],
  },
  {
    title: "Local runtimes & gateways",
    ids: [
      "Ollama", "LmStudio", "Vllm", "HuggingFace", "Together",
      "Groq", "Fireworks", "Replicate", "OpenRouter", "SiliconCloud",
    ],
  },
  {
    title: "Media generation",
    ids: [
      "Midjourney", "Stability", "Flux", "Runway", "Sora",
      "Kling", "Pika", "Suno", "ElevenLabs",
    ],
  },
  {
    title: "Dev tools",
    ids: [
      "Cursor", "Windsurf", "Cline", "Codex", "Copilot",
      "GithubCopilot", "Trae", "RooCode", "LobeHub",
    ],
  },
];

export function IconShowcase() {
  const total = SECTIONS.reduce((n, s) => n + s.ids.length, 0);

  return (
    <div className="flex flex-col gap-4.5">
      <p className="m-0 mb-1 text-[12px] leading-[1.55] text-fg-muted">
        A curated set of {total} brand glyphs from{" "}
        <code className="rounded-xs bg-surface-2 px-1.5 py-px font-mono text-[11px] text-fg">@lobehub/icons</code>.
        Full catalogue: <em className="not-italic text-fg">Cmd + K → View: Icon Gallery</em>.
      </p>

      {SECTIONS.map((sec) => (
        <section key={sec.title} className="flex flex-col gap-2">
          <header className="flex items-baseline justify-between font-mono text-[11px] font-semibold text-fg-faint tracking-normal">
            <span>{sec.title}</span>
            <span className="font-mono text-fg-muted tabular-nums">{sec.ids.length}</span>
          </header>
          <div className="grid gap-1.5 [grid-template-columns:repeat(auto-fill,minmax(96px,1fr))]">
            {sec.ids.map((id) => (
              <ShowcaseCard key={id} id={id} />
            ))}
          </div>
        </section>
      ))}
    </div>
  );
}

function ShowcaseCard({ id }: { id: string }) {
  const Glyph = IconMap[id];
  const meta = TocById[id];
  const title = meta?.fullTitle ?? id;
  return (
    <div
      title={`${title} — ${id}`}
      className="flex flex-col items-center gap-1.5 rounded-md border border-line bg-surface px-2 pt-2.5 pb-2 cursor-default transition-[border-color,transform] duration-150 hover:border-[color-mix(in_srgb,var(--color-accent)_30%,var(--color-line))] hover:-translate-y-px"
    >
      <div className="grid h-[34px] w-[34px] place-items-center rounded-sm bg-surface-2 text-fg">
        {Glyph ? <Glyph size={22} /> : <span className="font-mono text-fg-faint">?</span>}
      </div>
      <div className="max-w-full truncate text-center text-[11px] font-medium text-fg">
        {title}
      </div>
    </div>
  );
}
