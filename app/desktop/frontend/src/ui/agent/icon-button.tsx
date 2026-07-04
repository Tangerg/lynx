import { cn } from "@/lib/utils";
import { Button, type ButtonProps, Icon, type IconName } from "@/ui";

interface AgentIconButtonProps extends Omit<ButtonProps, "children" | "variant" | "size"> {
  icon: IconName;
  size?: "sm" | "md";
  active?: boolean;
  iconSize?: number;
}

export function AgentIconButton({
  icon,
  size = "md",
  active,
  iconSize = size === "sm" ? 14 : 16,
  className,
  type = "button",
  ...props
}: AgentIconButtonProps) {
  return (
    <Button
      {...props}
      type={type}
      variant="ghost"
      size={size === "sm" ? "icon-sm" : "icon-md"}
      data-active={active ? "" : undefined}
      className={cn("data-[active]:bg-fg/[0.065] data-[active]:text-fg", className)}
    >
      <Icon name={icon} size={iconSize} strokeWidth={1.8} />
    </Button>
  );
}
