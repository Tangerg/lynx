import { Icon } from "@/components/common";
import { cn } from "@/lib/utils";

type Props = {
  visible: boolean;
  onClick: () => void;
};

// Floating "scroll to bottom" affordance. Renders inside the composer
// area (positioned above the composer card) so it never overlaps the
// last message, and stays out of the layout flow.
//
// Animates in/out via opacity + translateY rather than mount/unmount,
// so the user gets a soft reveal instead of a pop-in. When `visible`
// is false it's still in the DOM but pointer-events: none + opacity: 0.
export function JumpToBottomButton({ visible, onClick }: Props) {
  return (
    <button
      type="button"
      aria-label="跳到底部"
      title="跳到底部"
      onClick={onClick}
      tabIndex={visible ? 0 : -1}
      className={cn(
        "absolute bottom-24 right-7 z-[3] grid h-9 w-9 place-items-center rounded-full",
        "bg-surface-2 text-fg border border-[color-mix(in_srgb,var(--color-text)_14%,transparent)]",
        "shadow-md cursor-pointer transition-all duration-150 ease-out",
        "hover:bg-surface-3 hover:border-[color-mix(in_srgb,var(--color-text)_22%,transparent)]",
        "active:translate-y-0 active:scale-95",
        visible
          ? "opacity-100 translate-y-0 pointer-events-auto"
          : "opacity-0 translate-y-2 pointer-events-none",
      )}
    >
      <Icon name="arrow-down" size={14} />
    </button>
  );
}
