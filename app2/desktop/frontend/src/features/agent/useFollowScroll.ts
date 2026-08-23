import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type RefObject,
  type UIEventHandler,
} from "react";

interface FollowScrollController {
  viewportRef: RefObject<HTMLDivElement | null>;
  contentRef: RefObject<HTMLDivElement | null>;
  following: boolean;
  hasNewMaterial: boolean;
  onScroll: UIEventHandler<HTMLDivElement>;
  follow(): void;
  escape(): void;
}

export function useFollowScroll(
  materialVersion: string,
  thresholdPixels: number,
  active = true,
): FollowScrollController {
  const viewportRef = useRef<HTMLDivElement>(null);
  const contentRef = useRef<HTMLDivElement>(null);
  const followsTail = useRef(true);
  const scheduledFrame = useRef<number | undefined>(undefined);
  const [following, setFollowing] = useState(true);
  const [hasNewMaterial, setHasNewMaterial] = useState(false);

  const scheduleTail = useCallback(() => {
    if (scheduledFrame.current !== undefined) {
      window.cancelAnimationFrame(scheduledFrame.current);
    }
    scheduledFrame.current = window.requestAnimationFrame(() => {
      scheduledFrame.current = undefined;
      const viewport = viewportRef.current;
      if (viewport === null || !followsTail.current) return;
      viewport.scrollTo({ top: viewport.scrollHeight });
    });
  }, []);
  const escape = useCallback(() => {
    followsTail.current = false;
    setFollowing(false);
  }, []);
  const follow = useCallback(() => {
    followsTail.current = true;
    setFollowing(true);
    setHasNewMaterial(false);
    scheduleTail();
  }, [scheduleTail]);
  const onScroll = useCallback<UIEventHandler<HTMLDivElement>>(
    (event) => {
      const viewport = event.currentTarget;
      const atTail =
        viewport.scrollHeight - viewport.scrollTop - viewport.clientHeight <
        thresholdPixels;
      followsTail.current = atTail;
      setFollowing((current) => (current === atTail ? current : atTail));
      if (atTail) setHasNewMaterial(false);
    },
    [thresholdPixels],
  );

  useLayoutEffect(() => {
    if (!active) return;
    if (followsTail.current) {
      const viewport = viewportRef.current;
      viewport?.scrollTo({ top: viewport.scrollHeight });
      scheduleTail();
      return;
    }
    setHasNewMaterial(true);
  }, [active, materialVersion, scheduleTail]);

  useEffect(() => {
    if (!active) return;
    const viewport = viewportRef.current;
    const content = contentRef.current;
    if (
      viewport === null ||
      content === null ||
      typeof ResizeObserver === "undefined"
    ) {
      return;
    }
    const observer = new ResizeObserver(() => {
      if (followsTail.current) {
        scheduleTail();
        return;
      }
      const atTail =
        viewport.scrollHeight - viewport.scrollTop - viewport.clientHeight <
        thresholdPixels;
      if (!atTail) return;
      followsTail.current = true;
      setFollowing(true);
      setHasNewMaterial(false);
    });
    observer.observe(viewport);
    observer.observe(content);
    return () => observer.disconnect();
  }, [active, scheduleTail, thresholdPixels]);

  useEffect(
    () => () => {
      if (scheduledFrame.current !== undefined) {
        window.cancelAnimationFrame(scheduledFrame.current);
      }
    },
    [],
  );

  return {
    viewportRef,
    contentRef,
    following,
    hasNewMaterial,
    onScroll,
    follow,
    escape,
  };
}
