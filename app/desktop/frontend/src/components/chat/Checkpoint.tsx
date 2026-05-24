import { Icon } from "@/components/common";

// Checkpoint divider — a "milestone reached" marker between message
// chunks. Horizontal flanking lines fade from transparent into a soft
// hairline at the centre, with the check glyph between.
export function Checkpoint({ text }: { text: string }) {
  return (
    <div
      className="my-2 flex items-center gap-3 font-mono text-[10.5px] font-semibold text-fg-faint
        before:flex-1 before:h-px before:content-[''] before:bg-[linear-gradient(90deg,transparent,var(--color-border-soft)_50%,transparent)]
        after:flex-1  after:h-px  after:content-[''] after:bg-[linear-gradient(90deg,transparent,var(--color-border-soft)_50%,transparent)]"
    >
      <div className="grid h-[18px] w-[18px] place-items-center rounded-full bg-surface-2 text-accent">
        <Icon name="check" size={11} strokeWidth={3} />
      </div>
      <span>{text}</span>
    </div>
  );
}
