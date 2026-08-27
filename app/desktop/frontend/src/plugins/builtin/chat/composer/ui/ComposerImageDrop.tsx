import { useEffect, useEffectEvent, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { imageFiles } from "@/plugins/builtin/chat/composer/public/input";
import { Icon } from "@/ui";
import { useT } from "@/lib/i18n";
import { hasComposerImageTransferItems } from "../application/composerInputEvents";

interface Props {
  enabled: boolean;
  onDropImages: (files: File[]) => void;
}

export function ComposerImageDrop({ enabled, onDropImages }: Props) {
  return enabled ? <EnabledComposerImageDrop onDropImages={onDropImages} /> : null;
}

function EnabledComposerImageDrop({ onDropImages }: Pick<Props, "onDropImages">) {
  const dragging = useWindowImageDrag(onDropImages);
  return dragging ? <ImageDropOverlay /> : null;
}

// Nested dragenter/dragleave events fire as the pointer crosses child elements,
// so depth is the overlay's flicker guard. preventDefault stays scoped to image
// drags, leaving unrelated native drop targets alone.
function useWindowImageDrag(onDropImages: (files: File[]) => void): boolean {
  const [dragging, setDragging] = useState(false);
  const depth = useRef(0);
  const deliverImages = useEffectEvent(onDropImages);

  useEffect(() => {
    const hasImageItems = (dt: DataTransfer | null | undefined): boolean =>
      hasComposerImageTransferItems(dt?.items);

    const onEnter = (event: DragEvent): void => {
      depth.current += 1;
      if (hasImageItems(event.dataTransfer)) setDragging(true);
    };
    const onLeave = (): void => {
      depth.current -= 1;
      if (depth.current <= 0) {
        depth.current = 0;
        setDragging(false);
      }
    };
    const onOver = (event: DragEvent): void => {
      if (hasImageItems(event.dataTransfer)) event.preventDefault();
    };
    const onDrop = (event: DragEvent): void => {
      depth.current = 0;
      setDragging(false);
      const files = imageFiles(event.dataTransfer?.files);
      if (files.length === 0) return;
      event.preventDefault();
      deliverImages(files);
    };

    window.addEventListener("dragenter", onEnter);
    window.addEventListener("dragleave", onLeave);
    window.addEventListener("dragover", onOver);
    window.addEventListener("drop", onDrop);
    return () => {
      window.removeEventListener("dragenter", onEnter);
      window.removeEventListener("dragleave", onLeave);
      window.removeEventListener("dragover", onOver);
      window.removeEventListener("drop", onDrop);
      depth.current = 0;
    };
  }, []);

  return dragging;
}

function ImageDropOverlay() {
  const t = useT();
  return createPortal(
    <div className="fixed inset-0 z-[var(--layer-modal)] grid place-items-center bg-scrim p-10">
      <div className="animate-rise-in flex flex-col items-center gap-3 rounded-composer border-2 border-dashed border-field-strong bg-canvas px-14 py-12 shadow-[var(--shadow-modal)]">
        <Icon name="image" size="xl" className="text-fg-muted" />
        <span className="text-ui-md font-medium text-fg-soft">{t("composer.drop.images")}</span>
      </div>
    </div>,
    document.body,
  );
}
