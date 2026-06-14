import { Icon } from "@/components/common";
import { useT } from "@/lib/i18n";

// Shared footer with a single "open the full view" affordance. `label` is an
// i18n key resolved here, so callsites pass a key (a literal still passes
// through t() unchanged) — one useT instead of one per preview component.
export function PreviewFoot({ label, onClick }: { label: string; onClick: () => void }) {
  const t = useT();
  return (
    <div className="mt-2.5 pt-2 text-right">
      <button
        type="button"
        onClick={onClick}
        className="inline-flex items-center gap-1.5 rounded-full border border-line bg-transparent px-2.5 py-1 font-sans text-[11px] font-semibold text-fg-muted transition-[background,border-color,color] hover:border-line-soft hover:bg-surface hover:text-fg"
      >
        {t(label)} <Icon name="share" size={11} />
      </button>
    </div>
  );
}
