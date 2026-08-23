import {
  Children,
  Component,
  cloneElement,
  isValidElement,
  memo,
  useEffect,
  useState,
  type ReactElement,
  type ReactNode,
} from "react";
import ReactMarkdown, { type Components } from "react-markdown";
import rehypeRaw from "rehype-raw";
import rehypeSanitize, { defaultSchema } from "rehype-sanitize";
import remarkCJKFriendly from "remark-cjk-friendly";
import remarkCJKFriendlyGFMStrikethrough from "remark-cjk-friendly-gfm-strikethrough";
import remarkGfm from "remark-gfm";
import remarkMath from "remark-math";

import {
  useLocalization,
  type Translate,
} from "../../localization/Localization";
import { CodeBlock } from "./CodeBlock";
import { MermaidBlock } from "./MermaidBlock";
import "./content.css";

interface MarkdownContentProps {
  source: string;
  highlight: string;
}

const rawHTMLSchema = {
  ...defaultSchema,
  tagNames: [...(defaultSchema.tagNames ?? []), "details", "summary", "kbd"],
  attributes: {
    ...defaultSchema.attributes,
    code: [
      ...(defaultSchema.attributes?.code ?? []),
      ["className", /^language-[\w#+.-]+$/, "math-inline", "math-display"],
    ],
    li: [
      ...(defaultSchema.attributes?.li ?? []),
      ["className", "task-list-item"],
    ],
    ul: [
      ...(defaultSchema.attributes?.ul ?? []),
      ["className", "contains-task-list"],
    ],
  },
};

type KatexPlugin = (typeof import("rehype-katex"))["default"];
let katexEnhancement: Promise<KatexPlugin | undefined> | undefined;

export function MarkdownContent(props: MarkdownContentProps) {
  const { t } = useLocalization();
  return (
    <MarkdownBoundary {...props}>
      <MarkdownMaterial {...props} t={t} />
    </MarkdownBoundary>
  );
}

const MarkdownMaterial = memo(function MarkdownMaterial({
  source,
  highlight,
  t,
}: MarkdownContentProps & { t: Translate }) {
  const [katexPlugin, setKatexPlugin] = useState<KatexPlugin>();
  const hasMath =
    source.includes("$") || source.includes("\\[") || source.includes("\\(");

  useEffect(() => {
    if (!hasMath || katexPlugin !== undefined) return;
    let current = true;
    void loadKatex().then((plugin) => {
      if (current && plugin !== undefined) setKatexPlugin(() => plugin);
    });
    return () => {
      current = false;
    };
  }, [hasMath, katexPlugin]);

  if (source === "") return null;
  return (
    <ReactMarkdown
      remarkPlugins={[
        remarkGfm,
        remarkCJKFriendly,
        remarkCJKFriendlyGFMStrikethrough,
        remarkMath,
      ]}
      rehypePlugins={
        katexPlugin
          ? [rehypeRaw, [rehypeSanitize, rawHTMLSchema], katexPlugin]
          : [rehypeRaw, [rehypeSanitize, rawHTMLSchema]]
      }
      urlTransform={safeMarkdownURL}
      components={componentsWithHighlight(highlight, t)}
    >
      {source}
    </ReactMarkdown>
  );
});

class MarkdownBoundary extends Component<
  MarkdownContentProps & { children: ReactNode },
  { failed: boolean }
> {
  state = { failed: false };

  static getDerivedStateFromError() {
    return { failed: true };
  }

  componentDidUpdate(previous: MarkdownContentProps) {
    if (previous.source !== this.props.source && this.state.failed) {
      this.setState({ failed: false });
    }
  }

  render() {
    if (this.state.failed) {
      return (
        <p className="markdown-fallback">
          {highlightedText(this.props.source, this.props.highlight)}
        </p>
      );
    }
    return this.props.children;
  }
}

function componentsWithHighlight(highlight: string, t: Translate): Components {
  return {
    a({ node: _node, href, children, ...props }) {
      const external =
        href?.startsWith("http://") || href?.startsWith("https://");
      return (
        <a
          {...props}
          href={href}
          target={external ? "_blank" : undefined}
          rel={external ? "noreferrer noopener" : undefined}
        >
          {highlightNodes(children, highlight)}
        </a>
      );
    },
    blockquote({ node: _node, children, ...props }) {
      return (
        <blockquote {...props}>
          {highlightNodes(children, highlight)}
        </blockquote>
      );
    },
    code({ node: _node, className, children, ...props }) {
      const source = String(children);
      const language = /(?:^|\s)language-([\w#+.-]+)/.exec(
        className ?? "",
      )?.[1];
      const block = language !== undefined || source.endsWith("\n");
      if (!block) {
        return (
          <code {...props} className={className}>
            {children}
          </code>
        );
      }
      const code = source.replace(/\n$/, "");
      return language?.toLocaleLowerCase() === "mermaid" ? (
        <MermaidBlock source={code} />
      ) : (
        <CodeBlock code={code} language={language} />
      );
    },
    h1({ node: _node, children, ...props }) {
      return <h1 {...props}>{highlightNodes(children, highlight)}</h1>;
    },
    h2({ node: _node, children, ...props }) {
      return <h2 {...props}>{highlightNodes(children, highlight)}</h2>;
    },
    h3({ node: _node, children, ...props }) {
      return <h3 {...props}>{highlightNodes(children, highlight)}</h3>;
    },
    h4({ node: _node, children, ...props }) {
      return <h4 {...props}>{highlightNodes(children, highlight)}</h4>;
    },
    img({ node: _node, src, alt }) {
      if (!src) return null;
      return (
        <a
          className="markdown-image-link"
          href={src}
          target="_blank"
          rel="noreferrer noopener"
        >
          {alt || t("markdown.openLinkedImage")}
        </a>
      );
    },
    li({ node: _node, children, ...props }) {
      return <li {...props}>{highlightNodes(children, highlight)}</li>;
    },
    p({ node: _node, children, ...props }) {
      return <p {...props}>{highlightNodes(children, highlight)}</p>;
    },
    pre({ node: _node, children }) {
      return <>{children}</>;
    },
    table({ node: _node, children, ...props }) {
      return (
        <div className="markdown-table-scroll" tabIndex={0}>
          <table {...props}>{children}</table>
        </div>
      );
    },
    td({ node: _node, children, ...props }) {
      return <td {...props}>{highlightNodes(children, highlight)}</td>;
    },
    th({ node: _node, children, ...props }) {
      return <th {...props}>{highlightNodes(children, highlight)}</th>;
    },
  };
}

async function loadKatex() {
  katexEnhancement ??= Promise.all([
    import("rehype-katex"),
    import("katex/dist/katex.min.css"),
  ])
    .then(([module]) => module.default)
    .catch(() => undefined);
  return katexEnhancement;
}

function highlightNodes(children: ReactNode, query: string): ReactNode {
  if (query === "") return children;
  return Children.map(children, (child) => {
    if (typeof child === "string") return highlightedText(child, query);
    if (
      !isValidElement(child) ||
      child.type === "code" ||
      child.type === "mark"
    ) {
      return child;
    }
    const element = child as ReactElement<{ children?: ReactNode }>;
    if (element.props.children === undefined) return child;
    return cloneElement(element, {
      children: highlightNodes(element.props.children, query),
    });
  });
}

function highlightedText(text: string, query: string) {
  const source = text.toLocaleLowerCase();
  const normalizedQuery = query.toLocaleLowerCase();
  const fragments: ReactNode[] = [];
  let cursor = 0;
  let match = source.indexOf(normalizedQuery);
  while (match >= 0) {
    if (match > cursor) fragments.push(text.slice(cursor, match));
    fragments.push(
      <mark key={`${match}:${fragments.length}`}>
        {text.slice(match, match + normalizedQuery.length)}
      </mark>,
    );
    cursor = match + normalizedQuery.length;
    match = source.indexOf(normalizedQuery, cursor);
  }
  if (cursor < text.length) fragments.push(text.slice(cursor));
  return fragments.length === 0 ? text : fragments;
}

function safeMarkdownURL(value: string, key: string) {
  const url = value.trim();
  const hasControlCharacter = Array.from(url).some((character) => {
    const code = character.charCodeAt(0);
    return code <= 0x1f || code === 0x7f;
  });
  if (url === "" || hasControlCharacter) return "";
  if (key === "src") {
    try {
      const parsed = new URL(url);
      return parsed.protocol === "https:" ? parsed.href : "";
    } catch {
      return "";
    }
  }
  if (url.startsWith("#")) return url;
  try {
    const parsed = new URL(url);
    return ["http:", "https:", "mailto:"].includes(parsed.protocol)
      ? parsed.href
      : "";
  } catch {
    return "";
  }
}
