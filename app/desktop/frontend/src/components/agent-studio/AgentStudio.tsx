import type { ComponentPropsWithoutRef, ReactNode } from "react";
import { cn } from "@/lib/utils";
import { Icon, type IconName } from "@/components/common/Icon";
import { StudioButton, type StudioButtonProps } from "./BasePrimitives";

interface AgentButtonProps extends Omit<StudioButtonProps, "children"> {
  children?: ReactNode;
}

interface AgentRowProps extends AgentButtonProps {
  active?: boolean;
  icon?: IconName;
  iconClassName?: string;
  trailing?: ReactNode;
  indent?: "none" | "nested";
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
    <StudioButton
      {...props}
      type={type}
      data-active={active ? "" : undefined}
      className={cn(
        "group flex h-7 w-full items-center gap-2 rounded-[7px] border-0 bg-transparent text-left",
        "text-[13px] leading-none text-fg-soft transition-[background-color,color,scale] duration-100 ease-out",
        "hover:bg-fg/[0.045] hover:text-fg focus-visible:bg-fg/[0.06] focus-visible:text-fg focus-visible:outline-none",
        "active:scale-[0.99] data-[active]:bg-fg/[0.075] data-[active]:text-fg",
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
    </StudioButton>
  );
}

interface AgentIconButtonProps extends AgentButtonProps {
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
    <StudioButton
      {...props}
      type={type}
      data-active={active ? "" : undefined}
      className={cn(
        "grid place-items-center rounded-[8px] border-0 bg-transparent text-fg-muted",
        "transition-[background-color,color,scale] duration-[120ms] ease-out",
        "hover:bg-fg/[0.045] hover:text-fg focus-visible:bg-fg/[0.06] focus-visible:text-fg focus-visible:outline-none",
        "active:scale-[0.96] data-[active]:bg-fg/[0.065] data-[active]:text-fg",
        size === "sm" ? "h-7 w-7" : "h-8 w-8",
        className,
      )}
    >
      <Icon name={icon} size={iconSize} strokeWidth={1.8} />
    </StudioButton>
  );
}

interface AgentToolbarButtonProps extends AgentButtonProps {
  icon?: IconName;
  trailingIcon?: IconName;
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
    <StudioButton
      {...props}
      type={type}
      className={cn(
        "inline-flex h-8 shrink-0 items-center gap-1.5 rounded-[8px] border-0 bg-transparent px-2.5",
        "font-sans text-[12.5px] font-medium leading-none text-fg-muted whitespace-nowrap",
        "shadow-[inset_0_0_0_0.5px_var(--color-field)] transition-[background-color,color,scale] duration-[120ms] ease-out",
        "hover:bg-fg/[0.045] hover:text-fg focus-visible:bg-fg/[0.06] focus-visible:text-fg focus-visible:outline-none",
        "active:scale-[0.96] disabled:cursor-not-allowed disabled:opacity-45",
        className,
      )}
    >
      {icon && <Icon name={icon} size={15} strokeWidth={1.8} className="shrink-0" />}
      {children}
      {trailingIcon && (
        <Icon name={trailingIcon} size={13} strokeWidth={1.8} className="shrink-0 text-fg-faint" />
      )}
    </StudioButton>
  );
}

export function AgentSectionLabel({
  children,
  className,
}: {
  children: ReactNode;
  className?: string;
}) {
  return (
    <div
      className={cn(
        "px-2.5 pb-1 pt-4 font-sans text-[11px] font-medium leading-none text-fg-faint",
        className,
      )}
    >
      {children}
    </div>
  );
}

export function AgentKbd({ children }: { children: ReactNode }) {
  return (
    <kbd className="inline-flex h-4 min-w-4 items-center justify-center rounded-[4px] bg-surface-2 px-1 font-mono text-[10.5px] font-medium leading-none text-fg-faint shadow-[inset_0_0_0_0.5px_var(--color-field)]">
      {children}
    </kbd>
  );
}

export function AgentStatusPill({
  children,
  tone = "neutral",
}: {
  children: ReactNode;
  tone?: "neutral" | "running" | "warning" | "success";
}) {
  const dotClass =
    tone === "running"
      ? "bg-accent"
      : tone === "warning"
        ? "bg-warning"
        : tone === "success"
          ? "bg-success"
          : "bg-fg-faint";
  return (
    <span className="inline-flex h-[22px] items-center gap-1.5 rounded-full bg-surface px-2.5 font-sans text-[11px] font-medium leading-none text-fg-muted shadow-[inset_0_0_0_0.5px_var(--color-field)]">
      <span className={cn("h-1.5 w-1.5 rounded-full", dotClass)} />
      {children}
    </span>
  );
}

export function AgentWindowControls() {
  return (
    <div className="flex h-[52px] shrink-0 items-center gap-2 px-4">
      <span className="h-3 w-3 rounded-full bg-[#ff5f57]" />
      <span className="h-3 w-3 rounded-full bg-[#febc2e]" />
      <span className="h-3 w-3 rounded-full bg-[#28c840]" />
    </div>
  );
}

export function AgentComposerSurface({
  className,
  children,
  ...props
}: ComponentPropsWithoutRef<"div">) {
  return (
    <div
      {...props}
      className={cn(
        "rounded-[18px] bg-canvas px-6 py-4 shadow-[var(--shadow-composer)]",
        "transition-[box-shadow] duration-[160ms] ease-out focus-within:shadow-[var(--shadow-popover)]",
        className,
      )}
    >
      {children}
    </div>
  );
}

export function AgentSurface({ className, children, ...props }: ComponentPropsWithoutRef<"div">) {
  return (
    <div
      {...props}
      className={cn(
        "rounded-[12px] bg-surface shadow-[inset_0_0_0_0.5px_var(--color-field)]",
        className,
      )}
    >
      {children}
    </div>
  );
}
