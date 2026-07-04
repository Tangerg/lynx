import type { ComponentPropsWithoutRef, ReactNode } from "react";
import { Button as BaseButton } from "@base-ui/react/button";
import { Tabs as BaseTabs } from "@base-ui/react/tabs";
import { cn } from "@/lib/utils";

export type StudioButtonProps = ComponentPropsWithoutRef<typeof BaseButton> & {
  children?: ReactNode;
};

export function StudioButton({
  className,
  type = "button",
  children,
  ...props
}: StudioButtonProps) {
  return (
    <BaseButton
      {...props}
      type={type}
      className={cn(
        "border-0 bg-transparent font-sans text-left focus-visible:outline-none disabled:cursor-not-allowed disabled:opacity-45",
        className,
      )}
    >
      {children}
    </BaseButton>
  );
}

export const StudioTabs = {
  Root: BaseTabs.Root,
  List: BaseTabs.List,
  Tab: BaseTabs.Tab,
  Panel: BaseTabs.Panel,
};
