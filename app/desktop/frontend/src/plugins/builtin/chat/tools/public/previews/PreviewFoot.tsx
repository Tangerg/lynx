import { Icon } from "@/ui";
import { useT } from "@/lib/i18n";

// Shared footer with a single "open the full view" affordance. `label` is an
// i18n key resolved here, so callsites pass a key (a literal still passes
// through t() unchanged) — one useT instead of one per preview component.
export function PreviewFoot({ label, onClick }: { label: string; onClick?: () => void }) {
  const t = useT();
  // No view to open (search / glob / lsp / skill / …) → render no foot, rather
  // than a button that does nothing on click.
  if (!onClick) return null;
  return (
    <div className="mt-2 pt-1.5 text-right">
      <button
        type="button"
        onClick={onClick}
        className="inline-flex items-center gap-1.5 rounded-md bg-transparent px-2.5 py-1 font-sans text-[11px] font-medium text-fg-muted transition-colors hover:bg-fg/[0.06] hover:text-fg"
      >
        {t(label)} <Icon name="share" size={11} />
      </button>
    </div>
  );
}
