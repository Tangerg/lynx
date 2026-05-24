import type { Components } from "react-markdown";
import { MermaidBlock } from "@/components/chat/MermaidBlock";
import { ShikiCodeBlock } from "@/components/chat/ShikiCodeBlock";

// react-markdown component overrides. `pre` unwraps because `code`
// below emits its own block container (would otherwise double-wrap).
// `a` opens external links in the OS browser — clicking a link inside
// the Wails WebView would otherwise navigate the chrome-less window.
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
