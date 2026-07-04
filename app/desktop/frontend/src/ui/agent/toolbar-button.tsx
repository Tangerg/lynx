import type { ReactNode } from "react";
import { cn } from "@/lib/utils";
import { Button, type ButtonProps, Icon, type IconName } from "@/ui";

interface AgentToolbarButtonProps extends Omit<
  ButtonProps,
  "children" | "variant" | "size" | "press"
> {
  icon?: IconName;
  trailingIcon?: IconName;
  children?: ReactNode;
}

export function AgentToolbarButton({
  icon,
  trailingIcon,
  className,
  children,
  type = "button",
  ...props
}: AgentToolbarButtonProps) {
  return (
    <Button
      {...props}
      type={type}
      variant="outline"
      size="md"
      className={cn("px-2.5 text-[12.5px]", className)}
    >
      {icon && <Icon name={icon} size={15} strokeWidth={1.8} className="shrink-0" />}
      {children}
      {trailingIcon && (
        <Icon name={trailingIcon} size={13} strokeWidth={1.8} className="shrink-0 text-fg-faint" />
      )}
    </Button>
  );
}
