import { useState } from "react";
import { Icon } from "@/components/common/Icon";
import { highlightCode } from "@/utils/highlight";

export function CodeBlock({ lang, file, text }: { lang: string; file: string; text: string }) {
  const [copied, setCopied] = useState(false);
  const onCopy = () => {
    try { navigator.clipboard?.writeText(text); } catch { /* ignore */ }
    setCopied(true);
    setTimeout(() => setCopied(false), 1500);
  };
  return (
    <div className="code-block">
      <div className="code-block-head">
        <span className="lang">{lang || "text"}</span>
        <span className="fname">{file || ""}</span>
        <button className="copy" onClick={onCopy}>
          <Icon name={copied ? "check" : "file"} size={11} />
          {copied ? "Copied" : "Copy"}
        </button>
      </div>
      <pre dangerouslySetInnerHTML={{ __html: highlightCode(text) }} />
    </div>
  );
}
