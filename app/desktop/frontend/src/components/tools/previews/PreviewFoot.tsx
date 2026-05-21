import { Icon } from "@/components/common";

// Shared footer with a single "open the full view" affordance.
export function PreviewFoot({ label, onClick }: { label: string; onClick: () => void }) {
  return (
    <div className="preview-foot">
      <button className="preview-open" onClick={onClick}>
        {label} <Icon name="share" size={11} />
      </button>
    </div>
  );
}
