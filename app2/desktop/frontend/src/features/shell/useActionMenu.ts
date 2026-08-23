import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type RefObject,
} from "react";

export interface ActionMenuController<
  Root extends HTMLElement,
  Trigger extends HTMLElement,
  Menu extends HTMLElement,
> {
  open: boolean;
  rootRef: RefObject<Root | null>;
  triggerRef: RefObject<Trigger | null>;
  menuRef: RefObject<Menu | null>;
  setOpen(open: boolean): void;
  toggle(): void;
  close(options?: { restoreFocus?: boolean }): void;
}

export function useActionMenu<
  Root extends HTMLElement = HTMLElement,
  Trigger extends HTMLElement = HTMLButtonElement,
  Menu extends HTMLElement = HTMLElement,
>(): ActionMenuController<Root, Trigger, Menu> {
  const [open, setOpen] = useState(false);
  const rootRef = useRef<Root>(null);
  const triggerRef = useRef<Trigger>(null);
  const menuRef = useRef<Menu>(null);
  const close = useCallback((options?: { restoreFocus?: boolean }) => {
    setOpen(false);
    if (options?.restoreFocus) {
      window.requestAnimationFrame(() => triggerRef.current?.focus());
    }
  }, []);
  const toggle = useCallback(() => setOpen((current) => !current), []);

  useEffect(() => {
    if (!open) return;
    const focusFrame = window.requestAnimationFrame(() => {
      menuItems(menuRef.current)[0]?.focus();
    });
    const onPointerDown = (event: PointerEvent) => {
      if (!rootRef.current?.contains(event.target as Node)) close();
    };
    const onFocusIn = (event: FocusEvent) => {
      if (!rootRef.current?.contains(event.target as Node)) close();
    };
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.isComposing || event.keyCode === 229) return;
      if (event.key === "Escape") {
        event.preventDefault();
        event.stopPropagation();
        close({ restoreFocus: true });
        return;
      }
      if (!menuRef.current?.contains(event.target as Node)) return;
      const items = menuItems(menuRef.current);
      if (items.length === 0) return;
      const current = Math.max(
        0,
        items.indexOf(document.activeElement as HTMLElement),
      );
      let next: number | undefined;
      if (event.key === "ArrowDown") next = (current + 1) % items.length;
      else if (event.key === "ArrowUp")
        next = (current - 1 + items.length) % items.length;
      else if (event.key === "Home") next = 0;
      else if (event.key === "End") next = items.length - 1;
      if (next === undefined) return;
      event.preventDefault();
      items[next]?.focus();
    };
    document.addEventListener("pointerdown", onPointerDown);
    document.addEventListener("focusin", onFocusIn);
    document.addEventListener("keydown", onKeyDown);
    return () => {
      window.cancelAnimationFrame(focusFrame);
      document.removeEventListener("pointerdown", onPointerDown);
      document.removeEventListener("focusin", onFocusIn);
      document.removeEventListener("keydown", onKeyDown);
    };
  }, [close, open]);

  return {
    open,
    rootRef,
    triggerRef,
    menuRef,
    setOpen,
    toggle,
    close,
  };
}

function menuItems(menu: HTMLElement | null): HTMLElement[] {
  return [
    ...(menu?.querySelectorAll<HTMLElement>(
      '[role="menuitem"]:not([disabled])',
    ) ?? []),
  ].filter(
    (item) => !item.hidden && item.getAttribute("aria-hidden") !== "true",
  );
}
