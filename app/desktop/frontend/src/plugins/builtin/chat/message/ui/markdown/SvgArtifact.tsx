import { useMemo } from "react";
import { ShikiCodeBlock } from "@/ui";
import { useT } from "@/lib/i18n";

export function SvgArtifact({ code, lang }: { code: string; lang: string }) {
  const t = useT();
  const src = useMemo(() => `data:image/svg+xml;charset=utf-8,${encodeURIComponent(code)}`, [code]);

  return (
    <ShikiCodeBlock
      lang={lang}
      code={code}
      preview={
        <img
          src={src}
          alt={t("message.svg.generatedAlt")}
          className="block max-h-96 w-full object-contain"
        />
      }
    />
  );
}
