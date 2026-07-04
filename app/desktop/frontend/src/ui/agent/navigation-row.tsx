import type { ReactNode } from "react";
import { cn } from "@/lib/utils";
import { Button, type ButtonProps, Icon, type IconName } from "@/ui";

interface AgentRowProps extends Omit<ButtonProps, "children" | "variant" | "size" | "press"> {
  active?: boolean;
  icon?: IconName;
  iconClassName?: string;
  trailing?: ReactNode;
  indent?: "none" | "nested";
  children?: ReactNode;
}

export function AgentRow({
  active,
  icon,
  iconClassName,
  trailing,
  indent = "none",
  className,
  children,
  type = "button",
  ...props
}: AgentRowProps) {
  return (
    <Button
      {...props}
      type={type}
      variant="ghost"
      size="sm"
      press={false}
      data-active={active ? "" : undefined}
      className={cn(
        "group h-7 w-full justify-start gap-2 rounded-[7px] text-left text-[13px] text-fg-soft",
        "transition-[background-color,color] duration-100",
        "focus-visible:bg-fg/[0.06] focus-visible:text-fg data-[active]:bg-fg/[0.075] data-[active]:text-fg",
        indent === "nested" ? "px-2.5 pl-7" : "px-2.5",
        className,
      )}
    >
      {icon && (
        <Icon
          name={icon}
          size={15}
          strokeWidth={1.8}
          className={cn("shrink-0 text-fg-muted group-data-[active]:text-fg", iconClassName)}
        />
      )}
      <span className="min-w-0 flex-1 truncate">{children}</span>
      {trailing}
    </Button>
  );
}
