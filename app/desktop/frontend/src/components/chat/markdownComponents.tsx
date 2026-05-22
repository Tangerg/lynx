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
// - `table`: wrap in an overflow-x container so wide tables (think 8+
//   columns of bench data) scroll instead of bursting the message
//   column. The wrapper is the scroll surface; the table itself keeps
//   its natural width.
// - `a`: open external links in a new tab. With Wails, clicking a
//   <a href> in the WebView opens it INSIDE the chrome-less window with
//   no back button — same as a desktop app eating a popup. `_blank` +
//   `noopener` punts it to the OS default browser, which is what users
//   expect when they click a link in chat.
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
  table({ children, ...rest }) {
    return (
      <div className="md-table-wrap">
        <table {...rest}>{children}</table>
      </div>
    );
  },
  a({ href, children, ...rest }) {
    return (
      <a href={href} target="_blank" rel="noopener noreferrer" {...rest}>
        {children}
      </a>
    );
  },
};
