import { cn } from "@/lib/classNames";
import { IconButton, type IconName } from "@/ui";

// The message action bar's button: a library IconButton with the two things this
// bar adds — a rest tone quiet enough to sit under a message, and corners that
// follow the bubble it hangs off (pill on the user's, md on the assistant's).
// Four plugins contribute to this bar; before this they shared a class string,
// so each one still spelled out the element, the tooltip and the aria-label.
//
// `title` is optional for the one action that opens a menu: that trigger has to
// be the element the menu anchors to, so it composes this as its rendered
// element and puts the tooltip on the outside.
interface MessageActionButtonProps {
  icon: IconName;
  role: string;
  title?: string;
  onClick?: () => void;
  className?: string;
  "aria-label"?: string;
  "aria-pressed"?: boolean;
}

export function MessageActionButton({ role, className, ...props }: MessageActionButtonProps) {
  return (
    <IconButton
      {...props}
      iconSize="sm"
      size="sm"
      quiet
      className={cn(role === "user" ? "rounded-full" : "rounded-md", className)}
    />
  );
}
