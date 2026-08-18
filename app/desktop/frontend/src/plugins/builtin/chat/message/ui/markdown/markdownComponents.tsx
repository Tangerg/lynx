import type { Components } from "react-markdown";
import {
  Children,
  cloneElement,
  isValidElement,
  useEffect,
  useRef,
  type ComponentProps,
  type ReactElement,
  type ReactNode,
} from "react";
import { ExternalLink, RichTooltip, ShikiCodeBlock } from "@/ui";
import { cn } from "@/lib/classNames";
import { useCitations } from "../CitationContext";
import { FileRefLink } from "@/plugins/builtin/chat/file-references/public/FileRefLink";
import { MarkdownImage } from "./MarkdownImage";
import { MermaidBlock } from "./MermaidBlock";
import { MarkdownTable } from "./MarkdownTable";
import { SvgArtifact } from "./SvgArtifact";

// Local favicon stand-in — a domain-initial tile, mirroring the web-search
// result card badge. The desktop build must NOT fetch a remote favicon (e.g.
// google's `s2/favicons` endpoint): that would leak which sources the user is
// reading to a third party. The glyph is derived from the domain on-device.
function SourceFavicon({ domain }: { domain: string }) {
  const letter = (domain.replace(/^www\./, "")[0] ?? "?").toUpperCase();
  return (
    <span className="grid h-4 w-4 shrink-0 place-items-center rounded-2xs bg-surface-3 text-ui-2xs font-semibold text-fg-muted">
      {letter}
    </span>
  );
}

// Per-message citation lookup. CitationContext is scoped to the
// owning message so two messages with [1] markers don't collide.
function CitationBadge({ n, label }: { n: number; label: string }) {
  const citations = useCitations();
  const source = citations.find((c) => c.index === n);

  // Marker without a matching source (e.g. agent wrote [3] but only
  // 2 results in search block) renders as plain text — no tooltip.
  if (!source) {
    return (
      <sup className="cite-marker text-fg-faint" data-citation={n}>
        {label}
      </sup>
    );
  }

  return (
    <RichTooltip
      delay={200}
      side="top"
      sideOffset={6}
      className="max-w-[360px] px-3 py-2.5"
      trigger={
        <sup
          className="cite-marker cursor-help rounded-2xs bg-surface-2 px-1.5 py-px text-ui-sm font-medium text-fg-muted transition-colors hover:bg-cta hover:text-cta-text"
          data-citation={n}
        >
          {label}
        </sup>
      }
    >
      <div className="flex items-center gap-1.5">
        <SourceFavicon domain={source.domain} />
        <span className="truncate font-mono text-ui-sm text-fg-faint">{source.domain}</span>
      </div>
      <div className="mt-1.5 text-ui-md font-semibold text-fg leading-snug">{source.title}</div>
      <div className="mt-1 text-ui-sm text-fg-muted leading-snug line-clamp-3">
        {source.snippet}
      </div>
    </RichTooltip>
  );
}

// Agent-emitted <style> blocks go through a Shadow DOM so their
// rules can't escape and clobber the host stylesheet. Pairs with
// rehype-raw, which is what lets the tag survive sanitization in
// the first place.
function ShadowStyleBlock({ children }: { children?: React.ReactNode }) {
  const hostRef = useRef<HTMLSpanElement>(null);
  const css = typeof children === "string" ? children : "";

  useEffect(() => {
    const host = hostRef.current;
    if (!host) return;
    const shadow = host.shadowRoot ?? host.attachShadow({ mode: "open" });
    shadow.innerHTML = `<style>${css}</style>`;
  }, [css]);

  return <span ref={hostRef} style={{ display: "none" }} aria-hidden="true" />;
}

// `pre` unwraps because the `code` override below emits its own block
// container. `a` forces target=_blank because a click inside the
// Wails WebView would otherwise navigate the chrome-less window away
// from the app.
function visibleText(children: ReactNode): string {
  return Children.toArray(children)
    .map((child) => {
      if (typeof child === "string" || typeof child === "number") return String(child);
      return isValidElement<{ children?: ReactNode }>(child)
        ? visibleText(child.props.children)
        : "";
    })
    .join("");
}

const WHITESPACE_ONLY = /^\s*$/;
const HAN_TEXT = /\p{Script=Han}/u;

/** Return the paragraph's media children only when it contains no prose. The
 *  whitespace/`br` filtering mirrors Markdown's insignificant separators. */
type MarkdownImageElementProps = ComponentProps<typeof MarkdownImage> & {
  node?: { tagName?: string };
};

function imageOnlyParagraph(children: ReactNode): ReactElement<MarkdownImageElementProps>[] | null {
  const material = Children.toArray(children).filter(
    (child) =>
      !(typeof child === "string" && WHITESPACE_ONLY.test(child)) &&
      !(isValidElement(child) && child.type === "br"),
  );
  if (
    material.length === 0 ||
    !material.every(
      (child): child is ReactElement<MarkdownImageElementProps> =>
        isValidElement<MarkdownImageElementProps>(child) && child.props.node?.tagName === "img",
    )
  )
    return null;
  return material;
}

const sharedMarkdownComponents: Components = {
  p({ children }) {
    const images = imageOnlyParagraph(children);
    if (images && images.length > 1) {
      const galleryImages = images.map((image, index) =>
        cloneElement(image, { key: `${image.props.src ?? index}-${index}`, allowWide: true }),
      );
      return (
        <p className="md-media-paragraph md-media-grid" data-markdown-image-grid="true">
          {galleryImages}
        </p>
      );
    }
    if (images?.length === 1) {
      const image = images[0]!;
      const wideImage = cloneElement(image, { key: image.props.src ?? 0, allowWide: true });
      return (
        <p
          className="md-media-paragraph md-media-wide-block"
          data-wide-markdown-block="true"
          data-wide-markdown-block-kind="image"
        >
          {wideImage}
        </p>
      );
    }
    return (
      <p
        data-markdown-han-text={HAN_TEXT.test(visibleText(children)) ? "true" : undefined}
        dir="auto"
      >
        {children}
      </p>
    );
  },
  h1({ children }) {
    return <h1 dir="auto">{children}</h1>;
  },
  h2({ children }) {
    return <h2 dir="auto">{children}</h2>;
  },
  h3({ children }) {
    return <h3 dir="auto">{children}</h3>;
  },
  h4({ children }) {
    return <h4 dir="auto">{children}</h4>;
  },
  h5({ children }) {
    return <h5 dir="auto">{children}</h5>;
  },
  h6({ children }) {
    return <h6 dir="auto">{children}</h6>;
  },
  ul({ children, className }) {
    return (
      <ul className={className} dir="auto">
        {children}
      </ul>
    );
  },
  ol({ children, className, start }) {
    return (
      <ol className={className} dir="auto" start={start}>
        {children}
      </ol>
    );
  },
  blockquote({ children }) {
    return <blockquote dir="auto">{children}</blockquote>;
  },
  pre({ children }) {
    // react-markdown gives fenced blocks without an info string a plain
    // `<code>` child. The code renderer cannot distinguish that node from
    // inline code by className alone, but this parent can: only block code is
    // wrapped in `<pre>`. Keep every fence on the same code-block surface so
    // an unlabelled shell snippet still has scrolling, highlighting fallback,
    // and the copy affordance.
    const child = Children.toArray(children)[0];
    if (
      Children.count(children) === 1 &&
      isValidElement<{
        children?: React.ReactNode;
        className?: string;
        node?: { tagName?: string };
      }>(child) &&
      child.props.node?.tagName === "code" &&
      !child.props.className
    ) {
      const code = String(child.props.children ?? "").replace(/\n$/, "");
      return <ShikiCodeBlock lang="text" code={code} />;
    }
    return <>{children}</>;
  },
  code({ className, children }) {
    const cls = String(className ?? "");
    const match = /language-([\w+-]+)/.exec(cls);
    if (!match) {
      // Don't spread the rest props — react-markdown's passNode puts the hast
      // `node` in there, which would leak onto the DOM as node="[object Object]".
      return (
        <code className={cls} dir="ltr">
          {children}
        </code>
      );
    }
    // Regex has a capture group, so match[1] is defined when match is.
    const lang = match[1]!.toLowerCase();
    const codeStr = String(children ?? "").replace(/\n$/, "");
    if (lang === "mermaid") return <MermaidBlock code={codeStr} />;
    if (
      lang === "svg" ||
      ((lang === "xml" || lang === "html" || lang === "htm") &&
        /^\s*(?:<\?xml[^>]*>\s*)?<svg[\s>]/i.test(codeStr))
    )
      return <SvgArtifact code={codeStr} lang={lang} />;
    return <ShikiCodeBlock lang={lang} code={codeStr} />;
  },
  td({ children, className, align, colSpan, rowSpan, style }) {
    return (
      <td
        className={cn(
          className,
          /^\d+$/.test(visibleText(children).trim()) && "md-table-cell-numeric",
        )}
        align={align}
        colSpan={colSpan}
        rowSpan={rowSpan}
        style={style}
        dir="auto"
      >
        {children}
      </td>
    );
  },
  th({ children, className, align, colSpan, rowSpan, style }) {
    return (
      <th
        className={className}
        align={align}
        colSpan={colSpan}
        rowSpan={rowSpan}
        style={style}
        dir="auto"
      >
        {children}
      </th>
    );
  },
  img({ src, alt, title, ...rest }) {
    const allowWide = (rest as { allowWide?: boolean }).allowWide;
    return <MarkdownImage src={src} alt={alt} title={title} allowWide={allowWide} />;
  },
  a({ href, title, children, ...rest }) {
    // A `data-file-ref` anchor is emitted by rehypeFileRefs (not a real link) —
    // render it as a FileRefLink that opens the file viewer instead of
    // navigating. `data-file-line` is "0" / absent when no line was parsed.
    const r = rest as { "data-file-ref"?: string; "data-file-line"?: string };
    if (r["data-file-ref"]) {
      return <FileRefLink path={r["data-file-ref"]} line={Number(r["data-file-line"]) || 0} />;
    }
    // Forward only real anchor attrs (href/title); the rest carries the hast
    // `node`, which must not reach the DOM.
    return (
      <ExternalLink href={href} title={title}>
        {children}
      </ExternalLink>
    );
  },
  style({ children }) {
    return <ShadowStyleBlock>{children}</ShadowStyleBlock>;
  },
  // Only `<sup>` carrying `data-citation` (emitted by rehypeCitations)
  // becomes a CitationBadge; any other `<sup>` the user wrote by hand
  // passes through unchanged.
  sup({ children, ...rest }) {
    const ds = (rest as { "data-citation"?: string })["data-citation"];
    const n = Number(ds);
    // Only the rehypeCitations-emitted numeric data-citation becomes a badge; a
    // hand-authored `<sup data-citation="abc">` (n=NaN) falls through to plain.
    if (ds && Number.isInteger(n)) {
      const label = typeof children === "string" ? children : `[${ds}]`;
      return <CitationBadge n={n} label={label} />;
    }
    // No rest spread — keep the hast `node` off the DOM (see `code`).
    return <sup>{children}</sup>;
  },
};

/** React-markdown exposes source offsets on each table HAST node but does not
 *  pass the source string to a component. Bind the block's source once so each
 *  table can recover its exact Markdown slice without re-serialising the DOM. */
export function createMarkdownComponents(markdownSource: string): Components {
  return {
    ...sharedMarkdownComponents,
    table({ children, node }) {
      const start = node?.position?.start.offset;
      const end = node?.position?.end.offset;
      const tableSource =
        typeof start === "number" && typeof end === "number"
          ? markdownSource.slice(start, end)
          : markdownSource;
      return <MarkdownTable markdownSource={tableSource}>{children}</MarkdownTable>;
    },
  };
}
