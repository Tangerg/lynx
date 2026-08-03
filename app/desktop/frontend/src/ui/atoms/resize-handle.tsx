import type { KeyboardEvent as ReactKeyboardEvent, PointerEvent as ReactPointerEvent } from "react";
import { useCallback, useEffect, useRef } from "react";
import type { SeparatorPrimitiveProps } from "@/ui/primitives";
import { SeparatorPrimitive } from "@/ui/primitives";

/** Arrow nudge, and the coarse step Shift buys. */
const STEP_PX = 8;
const COARSE_STEP_PX = 24;

const RESIZE_KEYS = ["ArrowLeft", "ArrowRight", "Home", "End"];

/**
 * A draggable pane edge — the whole gesture, not only the separator's role.
 *
 * Base UI has no split-pane primitive, so the drag is ours to write; the point of writing
 * it once here is that a pane declares only what is different about it. What is the same
 * lives in this atom: listeners on the window (so a drag survives the cursor leaving a
 * 10px target), release on `pointercancel` as well as `pointerup`, arrow / Home / End
 * with a coarse Shift step, an ARIA range kept honest against the container's live width,
 * and no commit for a press that never moved.
 *
 * A resize writes the width straight onto the container as a custom property and tells
 * the caller once, at the end: React state per pointer-move re-renders the pane and
 * everything inside it at pointer frequency.
 */
export interface ResizeHandleProps extends Omit<
  SeparatorPrimitiveProps,
  "orientation" | "role" | "tabIndex" | "onPointerDown" | "onKeyDown" | "onKeyUp" | "onBlur"
> {
  /**
   * Which side of its pane the handle sits on. A handle on the pane's `end` edge grows it
   * as the pointer moves right; one on its `start` edge grows it as the pointer moves
   * left. This is the only reason the arithmetic differs between panes.
   */
  edge: "start" | "end";
  /**
   * The persisted width, which is what the handle announces — not what it could read back
   * from the container. A window resize clamps the layout without touching the
   * preference, so the two legitimately disagree, and announcing the preference against
   * current geometry keeps the order of two ResizeObserver callbacks from being
   * observable: reading the property could announce the previous viewport's clamp after
   * the layout had already been restored.
   */
  value: number;
  /** The element the width is set on, found from the handle. */
  container: (handle: HTMLElement) => HTMLElement | null;
  /** Custom property written on the container while resizing. */
  property: string;
  /**
   * The live width a gesture starts from. A callback rather than a read of `property`,
   * because a pane whose rendered width can be narrower than the property asked for must
   * start from what the user can see.
   */
  read: (container: HTMLElement) => number;
  minWidth: number;
  /** A pane is bounded by what is left for the columns beside it, so the ceiling follows
   *  the container rather than being a fixed number. */
  maxWidth: (containerWidth: number) => number;
  /** Persist. Once per gesture: on release, or per keyboard step. */
  onCommit: (width: number) => void;
  /** Set on the container while resizing, for a pane that animates its own width —
   *  otherwise every step starts an animation toward the width after it. */
  resizingAttribute?: string;
}

export function ResizeHandle({
  edge,
  value,
  container,
  property,
  read,
  minWidth,
  maxWidth,
  onCommit,
  resizingAttribute,
  ...props
}: ResizeHandleProps) {
  const handleRef = useRef<HTMLDivElement>(null);
  // Held so a gesture that ends without a pointer event — the component unmounting
  // mid-drag, or a release while the window was hidden — cannot leave `pointermove`
  // attached to the window.
  const listenersRef = useRef<{ move: (event: PointerEvent) => void; up: () => void } | null>(null);
  // The element currently carrying `resizingAttribute`, remembered rather than looked up
  // again: by the time an unmount clears it, the handle is detached and can no longer
  // find its own container.
  const markedRef = useRef<HTMLElement | null>(null);

  // The pane's declarations are read through a ref: the pointer handlers are installed
  // once per drag and must not hold the render that started it.
  const paneRef = useRef({ container, property, read, minWidth, maxWidth, onCommit });
  useEffect(() => {
    paneRef.current = { container, property, read, minWidth, maxWidth, onCommit };
  });

  const detach = useCallback(() => {
    const listeners = listenersRef.current;
    if (listeners) {
      window.removeEventListener("pointermove", listeners.move);
      window.removeEventListener("pointerup", listeners.up);
      window.removeEventListener("pointercancel", listeners.up);
      listenersRef.current = null;
    }
    const marked = markedRef.current;
    if (marked && resizingAttribute) marked.removeAttribute(resizingAttribute);
    markedRef.current = null;
  }, [resizingAttribute]);

  const mark = useCallback(
    (element: HTMLElement) => {
      if (!resizingAttribute) return;
      element.setAttribute(resizingAttribute, "");
      markedRef.current = element;
    },
    [resizingAttribute],
  );

  useEffect(() => detach, [detach]);

  useEffect(() => {
    const handle = handleRef.current;
    const element = handle ? paneRef.current.container(handle) : null;
    if (!handle || !element) return;
    const sync = () => {
      const pane = paneRef.current;
      const max = pane.maxWidth(element.clientWidth);
      handle.setAttribute("aria-valuemax", String(max));
      handle.setAttribute("aria-valuenow", String(clampWidth(value, pane.minWidth, max)));
    };
    sync();
    const observer = new ResizeObserver(sync);
    observer.observe(element);
    return () => observer.disconnect();
  }, [value]);

  const onPointerDown = useCallback(
    (event: ReactPointerEvent<HTMLDivElement>) => {
      const handle = handleRef.current;
      const element = handle ? paneRef.current.container(handle) : null;
      if (!handle || !element || event.button !== 0) return;
      event.preventDefault();
      detach();

      const startX = event.clientX;
      const startWidth = paneRef.current.read(element);
      let width = startWidth;
      let moved = false;

      const move = (moveEvent: PointerEvent) => {
        const pane = paneRef.current;
        const delta = edge === "end" ? moveEvent.clientX - startX : startX - moveEvent.clientX;
        if (delta !== 0) moved = true;
        // Re-read the container each move: a window resized mid-drag moves the ceiling.
        width = clampWidth(startWidth + delta, pane.minWidth, pane.maxWidth(element.clientWidth));
        element.style.setProperty(pane.property, `${width}px`);
        handle.setAttribute("aria-valuenow", String(width));
      };
      const up = () => {
        detach();
        // A press that never moved is not a resize. Committing one wrote the preference
        // back at its own value on every click of the handle.
        if (moved) paneRef.current.onCommit(width);
      };

      mark(element);
      listenersRef.current = { move, up };
      window.addEventListener("pointermove", move);
      window.addEventListener("pointerup", up);
      window.addEventListener("pointercancel", up);
    },
    [detach, edge, mark],
  );

  const onKeyDown = useCallback(
    (event: ReactKeyboardEvent<HTMLDivElement>) => {
      if (!RESIZE_KEYS.includes(event.key)) return;
      const handle = handleRef.current;
      const pane = paneRef.current;
      const element = handle ? pane.container(handle) : null;
      if (!handle || !element) return;
      event.preventDefault();

      const max = pane.maxWidth(element.clientWidth);
      const step = event.shiftKey ? COARSE_STEP_PX : STEP_PX;
      const grows = event.key === (edge === "end" ? "ArrowRight" : "ArrowLeft");
      const next =
        event.key === "Home"
          ? pane.minWidth
          : event.key === "End"
            ? max
            : clampWidth(pane.read(element) + (grows ? step : -step), pane.minWidth, max);

      // Held down, the arrow keys repeat every few tens of milliseconds, so the
      // suppression has to last until the key is released rather than for one step.
      mark(element);
      element.style.setProperty(pane.property, `${next}px`);
      handle.setAttribute("aria-valuemax", String(max));
      handle.setAttribute("aria-valuenow", String(next));
      pane.onCommit(next);
    },
    [edge, mark],
  );

  // Blur as well as key-up: focus can leave the handle while a key is still down, and the
  // key-up then lands somewhere else, leaving the pane unable to animate again. Only ever
  // clears the mark — a pointer drag holds its own listeners and is unaffected.
  const releaseKeyboard = useCallback(() => {
    if (listenersRef.current) return;
    detach();
  }, [detach]);

  return (
    <SeparatorPrimitive
      {...props}
      ref={handleRef}
      orientation="vertical"
      tabIndex={0}
      aria-valuemin={minWidth}
      aria-valuenow={Math.round(value)}
      onPointerDown={onPointerDown}
      onKeyDown={onKeyDown}
      onKeyUp={releaseKeyboard}
      onBlur={releaseKeyboard}
    />
  );
}

/** The same clamp the ARIA range announces, so the two can never disagree. */
function clampWidth(width: number, minWidth: number, maxWidth: number): number {
  return Math.round(Math.min(maxWidth, Math.max(minWidth, width)));
}
