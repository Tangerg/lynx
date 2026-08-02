import { Button, Icon } from "@/ui";
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
      <Button variant="ghost" size="xs" onClick={onClick}>
        {t(label)} <Icon name="share" size="xs" />
      </Button>
    </div>
  );
}
