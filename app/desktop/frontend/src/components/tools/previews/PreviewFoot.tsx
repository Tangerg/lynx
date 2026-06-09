import { Icon } from "@/components/common";

// Shared footer with a single "open the full view" affordance.
export function PreviewFoot({ label, onClick }: { label: string; onClick: () => void }) {
  return (
    <div className="mt-2.5 pt-2 text-right">
      <button
        type="button"
        onClick={onClick}
        className="inline-flex items-center gap-1.5 rounded-full border border-line bg-transparent px-2.5 py-1 font-sans text-[11px] font-semibold text-fg-muted transition-[background,border-color,color] hover:border-line-soft hover:bg-surface hover:text-fg"
      >
        {label} <Icon name="share" size={11} />
      </button>
    </div>
  );
}
