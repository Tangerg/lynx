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
    <div className="icon-showcase">
      <p className="icon-showcase-blurb">
        A curated set of {total} brand glyphs from <code>@lobehub/icons</code>.
        Full catalogue: <em>Cmd + K → View: Icon Gallery</em>.
      </p>

      {SECTIONS.map((sec) => (
        <section key={sec.title} className="icon-showcase-section">
          <header className="icon-showcase-head">
            <span>{sec.title}</span>
            <span className="icon-showcase-count">{sec.ids.length}</span>
          </header>
          <div className="icon-showcase-grid">
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
    <div className="icon-showcase-card" title={`${title} — ${id}`}>
      <div className="icon-showcase-glyph">
        {Glyph ? <Glyph size={22} /> : <span className="icon-showcase-missing">?</span>}
      </div>
      <div className="icon-showcase-name">{title}</div>
    </div>
  );
}
