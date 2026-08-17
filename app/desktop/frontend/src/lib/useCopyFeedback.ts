import { useCallback, useLayoutEffect, useRef, useState } from "react";
import { copyText } from "./clipboard";

interface CopyFeedbackOwnership {
  material: string;
  revision: number;
  mounted: boolean;
  resetTimer: ReturnType<typeof setTimeout> | undefined;
}

function clearResetTimer(owner: CopyFeedbackOwnership): void {
  if (owner.resetTimer === undefined) return;
  clearTimeout(owner.resetTimer);
  owner.resetTimer = undefined;
}

/** Inline clipboard feedback owned by one exact piece of visible material.
 *
 * Streaming output, code and run digests can all replace their text without
 * replacing the button component. Each copy intent therefore carries both the
 * material it copied and a monotonic revision. A retired or older clipboard
 * response may have changed the system clipboard, but it cannot publish
 * "copied" into the material that replaced it or extend a newer intent's timer.
 */
export function useCopyFeedback(
  material: string,
  resetAfterMs = 1500,
): { copied: boolean; copy: () => Promise<boolean> } {
  const ownerRef = useRef<CopyFeedbackOwnership>({
    material,
    revision: 0,
    mounted: true,
    resetTimer: undefined,
  });
  const [acceptedRevision, setAcceptedRevision] = useState<number | null>(null);

  // Layout ownership changes before the replacement material can paint or
  // receive an event. Promise continuations run only after this transition has
  // retired the previous revision.
  useLayoutEffect(() => {
    const owner = ownerRef.current;
    if (owner.material === material) return;
    owner.material = material;
    owner.revision += 1;
    clearResetTimer(owner);
    setAcceptedRevision(null);
  }, [material]);

  useLayoutEffect(() => {
    const owner = ownerRef.current;
    owner.mounted = true;
    return () => {
      owner.mounted = false;
      owner.revision += 1;
      clearResetTimer(owner);
    };
  }, []);

  const copy = useCallback(async (): Promise<boolean> => {
    const owner = ownerRef.current;
    const revision = ++owner.revision;
    clearResetTimer(owner);
    setAcceptedRevision(null);

    const accepted = await copyText(material);
    if (!accepted || !owner.mounted || owner.material !== material || owner.revision !== revision) {
      return false;
    }

    setAcceptedRevision(revision);
    owner.resetTimer = setTimeout(() => {
      owner.resetTimer = undefined;
      if (!owner.mounted || owner.material !== material || owner.revision !== revision) return;
      setAcceptedRevision(null);
    }, resetAfterMs);
    return true;
  }, [material, resetAfterMs]);

  const owner = ownerRef.current;
  return {
    copied:
      owner.material === material &&
      acceptedRevision !== null &&
      acceptedRevision === owner.revision,
    copy,
  };
}
