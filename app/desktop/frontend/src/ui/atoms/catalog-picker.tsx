import { useState } from "react";
import { cn } from "@/lib/classNames";
import { ComboboxPrimitive } from "@/ui/primitives";
import { Icon, type IconName } from "@/ui/icons";
import { buttonStyles } from "./button";
import { Popover } from "./popover";

export interface CatalogPickerItem {
  id: string;
  label: string;
  icon?: IconName;
  keywords?: readonly string[];
  active?: boolean;
}

export interface CatalogPickerGroup {
  id: string;
  label: string;
  items: CatalogPickerItem[];
}

/**
 * A searchable launcher for a grouped catalog of application surfaces.
 *
 * The trigger stays compact enough for desktop chrome while the popup carries
 * the catalog's real information architecture. Base UI owns filtering, arrow
 * navigation, focus return and selection; callers only provide translated
 * labels and handle the chosen item.
 */
export function CatalogPicker({
  groups,
  label,
  placeholder,
  emptyLabel,
  onSelect,
  className,
}: {
  groups: CatalogPickerGroup[];
  label: string;
  placeholder: string;
  emptyLabel: string;
  onSelect: (item: CatalogPickerItem) => void;
  className?: string;
}) {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");

  return (
    <Popover.Root
      open={open}
      onOpenChange={(nextOpen) => {
        setOpen(nextOpen);
        if (!nextOpen) setQuery("");
      }}
    >
      <Popover.Trigger
        aria-label={label}
        title={label}
        data-slot="button"
        data-variant="ghost"
        className={cn(
          buttonStyles({ variant: "ghost", size: "icon-sm" }),
          "data-[popup-open]:bg-selected data-[popup-open]:text-fg",
          className,
        )}
      >
        <Icon name="plus" size="sm" />
      </Popover.Trigger>

      <Popover.Content
        aria-label={label}
        align="end"
        sideOffset={6}
        className="w-[300px] max-w-[var(--available-width)] p-1.5"
      >
        <ComboboxPrimitive.Root<CatalogPickerItem>
          items={groups}
          value={null}
          inputValue={query}
          onInputValueChange={setQuery}
          onValueChange={(item) => {
            if (!item) return;
            onSelect(item);
            setOpen(false);
          }}
          itemToStringLabel={(item) => [item.label, ...(item.keywords ?? [])].join(" ")}
          autoHighlight
          inline
          open
        >
          <div className="mb-1 flex h-[var(--field-height-md)] items-center gap-2 rounded-[var(--field-radius)] border-[length:var(--control-edge-width)] border-field bg-canvas px-2.5 text-fg-muted focus-within:border-field-strong focus-within:text-fg">
            <Icon name="search" size="sm" className="shrink-0" />
            <ComboboxPrimitive.Input
              aria-label={placeholder}
              placeholder={placeholder}
              className="h-full min-w-0 flex-1 border-0 bg-transparent font-sans text-ui-md text-fg outline-none placeholder:text-fg-faint"
            />
          </div>

          <ComboboxPrimitive.Empty className="empty:hidden px-2.5 py-6 text-center text-ui-sm text-fg-faint">
            {emptyLabel}
          </ComboboxPrimitive.Empty>
          <ComboboxPrimitive.List className="max-h-[min(420px,var(--available-height))] overflow-y-auto overscroll-contain scroll-py-1 outline-none data-empty:p-0">
            {(group: CatalogPickerGroup) => (
              <ComboboxPrimitive.Group
                key={group.id}
                items={group.items}
                className="pb-1.5 last:pb-0"
              >
                <ComboboxPrimitive.GroupLabel className="px-2.5 pb-1 pt-2 text-ui-xs font-medium text-fg-faint select-none first:pt-1.5">
                  {group.label}
                </ComboboxPrimitive.GroupLabel>
                <ComboboxPrimitive.Collection>
                  {(item: CatalogPickerItem) => (
                    <ComboboxPrimitive.Item
                      key={item.id}
                      value={item}
                      className="grid min-h-9 cursor-default grid-cols-[16px_minmax(0,1fr)_14px] items-center gap-2 rounded-[var(--shape-sm)] px-2.5 text-ui-md text-fg outline-none select-none data-[highlighted]:bg-hover"
                    >
                      <Icon name={item.icon ?? "panel-r"} size="sm" className="text-fg-muted" />
                      <span className="truncate">{item.label}</span>
                      {item.active ? (
                        <Icon name="check" size="xs" className="text-accent" />
                      ) : (
                        <span />
                      )}
                    </ComboboxPrimitive.Item>
                  )}
                </ComboboxPrimitive.Collection>
              </ComboboxPrimitive.Group>
            )}
          </ComboboxPrimitive.List>
        </ComboboxPrimitive.Root>
      </Popover.Content>
    </Popover.Root>
  );
}
