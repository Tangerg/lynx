import { Icon } from "@/components/common";

type Props = {
  visible: boolean;
  onClick: () => void;
};

// Floating "scroll to bottom" affordance. Renders inside the composer
// area (positioned above the composer card via CSS) so it never overlaps
// the last message, and stays out of the layout flow.
//
// The button visually animates in/out via opacity + translateY rather
// than mount/unmount, so the user gets a soft reveal instead of a
// pop-in. When `visible` is false it's still in the DOM but
// pointer-events: none + opacity: 0.
export function JumpToBottomButton({ visible, onClick }: Props) {
  return (
    <button
      type="button"
      className={`jump-to-bottom ${visible ? "is-visible" : ""}`}
      aria-label="跳到底部"
      title="跳到底部"
      onClick={onClick}
      tabIndex={visible ? 0 : -1}
    >
      <Icon name="arrow-down" size={14} />
    </button>
  );
}
