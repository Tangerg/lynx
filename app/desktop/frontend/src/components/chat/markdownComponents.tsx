import type { Components } from "react-markdown";
import { MermaidBlock } from "@/components/chat/MermaidBlock";
import { ShikiCodeBlock } from "@/components/chat/ShikiCodeBlock";

// Component overrides for react-markdown.
//
// - `pre`: unwrap. Our `code` override below handles the entire block-
//   level rendering (Shiki / Mermaid / plain fallback) and emits its
//   own outer container — keeping the default `<pre>` would double-wrap.
// - `code`: route by language tag. Mermaid blocks render as diagrams,
//   everything else with a recognised `language-X` className goes
//   through Shiki. Inline code (no language- prefix) stays as a plain
//   `<code>` so it can sit in the text flow with .fade-in alongside it.
export const markdownComponents: Components = {
  pre({ children }) {
    return <>{children}</>;
  },
  code({ className, children, ...rest }) {
    const cls = String(className ?? "");
    const match = /language-([\w+-]+)/.exec(cls);
    if (!match) {
      return (
        <code className={cls} {...rest}>
          {children}
        </code>
      );
    }
    const lang = match[1].toLowerCase();
    const codeStr = String(children ?? "").replace(/\n$/, "");
    if (lang === "mermaid") return <MermaidBlock code={codeStr} />;
    return <ShikiCodeBlock lang={lang} code={codeStr} />;
  },
};
