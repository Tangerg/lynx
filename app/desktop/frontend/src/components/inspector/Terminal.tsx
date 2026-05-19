import type { TermLine } from "@/components/tools/previews";

export function Terminal({ lines, running }: { lines: TermLine[]; running: boolean }) {
  return (
    <div className="term">
      {lines.map((l, i) => (
        <span key={i} className={l.kind}>{l.text}</span>
      ))}
      {running && (
        <span style={{
          display: "inline-flex", alignItems: "center", gap: 8,
          marginTop: 8, color: "var(--color-info)",
        }}>
          <span style={{
            width: 8, height: 8, background: "currentColor",
            borderRadius: "50%", animation: "pulse 1.2s ease-in-out infinite",
          }} />
          tsc watching for changes…
        </span>
      )}
    </div>
  );
}
