import {
  useEffect,
  useRef,
  useState,
  type MouseEvent,
} from "react";
import { createPortal } from "react-dom";

import { saveImage } from "../../../runtime/desktopBridge";

export interface GalleryImage {
  key: string;
  source: string;
  alt: string;
}

export function ImageGallery({ images }: { images: GalleryImage[] }) {
  const [activeIndex, setActiveIndex] = useState<number>();
  if (images.length === 0) return null;
  const selectedIndex =
    activeIndex === undefined
      ? undefined
      : Math.min(activeIndex, images.length - 1);
  return (
    <>
      <div className="message-image-gallery" data-count={images.length}>
        {images.map((image, index) => (
          <button
            key={image.key}
            type="button"
            className="message-image-trigger"
            aria-label={`Open ${image.alt}`}
            onClick={() => setActiveIndex(index)}
          >
            <img src={image.source} alt={image.alt} loading="lazy" />
            <span aria-hidden="true">Expand</span>
          </button>
        ))}
      </div>
      {selectedIndex !== undefined ? (
        <ImageLightbox
          images={images}
          activeIndex={selectedIndex}
          onSelect={setActiveIndex}
          onClose={() => setActiveIndex(undefined)}
        />
      ) : null}
    </>
  );
}

function ImageLightbox({
  images,
  activeIndex,
  onSelect,
  onClose,
}: {
  images: GalleryImage[];
  activeIndex: number;
  onSelect(index: number): void;
  onClose(): void;
}) {
  const dialog = useRef<HTMLElement>(null);
  const closeButton = useRef<HTMLButtonElement>(null);
  const saveOperation = useRef(0);
  const [zoom, setZoom] = useState(1);
  const [saveState, setSaveState] = useState<
    "idle" | "saving" | "saved" | "canceled" | "failed"
  >("idle");
  const image = images[activeIndex];

  useEffect(() => {
    const previouslyFocused = document.activeElement as HTMLElement | null;
    closeButton.current?.focus();
    return () => previouslyFocused?.focus();
  }, []);

  useEffect(() => {
    setZoom(1);
    setSaveState("idle");
    saveOperation.current += 1;
  }, [activeIndex]);

  useEffect(
    () => () => {
      saveOperation.current += 1;
    },
    [],
  );

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        event.preventDefault();
        onClose();
      } else if (event.key === "ArrowLeft" && images.length > 1) {
        event.preventDefault();
        onSelect((activeIndex - 1 + images.length) % images.length);
      } else if (event.key === "ArrowRight" && images.length > 1) {
        event.preventDefault();
        onSelect((activeIndex + 1) % images.length);
      } else if (event.key === "Tab") {
        const controls = [
          ...(dialog.current?.querySelectorAll<HTMLElement>(
            'button:not(:disabled), [href], [tabindex]:not([tabindex="-1"])',
          ) ?? []),
        ];
        const first = controls[0];
        const last = controls.at(-1);
        if (first === undefined || last === undefined) return;
        if (event.shiftKey && document.activeElement === first) {
          event.preventDefault();
          last.focus();
        } else if (!event.shiftKey && document.activeElement === last) {
          event.preventDefault();
          first.focus();
        }
      }
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [activeIndex, images.length, onClose, onSelect]);

  const save = async () => {
    if (saveState === "saving") return;
    const operation = ++saveOperation.current;
    setSaveState("saving");
    try {
      const result = await saveImage(image.source);
      if (operation === saveOperation.current) {
        setSaveState(result.type === "saved" ? "saved" : "canceled");
      }
    } catch {
      if (operation === saveOperation.current) setSaveState("failed");
    }
  };

  const stopBackdrop = (event: MouseEvent) => event.stopPropagation();

  return createPortal(
    <div className="image-lightbox-backdrop" onMouseDown={onClose}>
      <section
        ref={dialog}
        className="image-lightbox"
        role="dialog"
        aria-modal="true"
        aria-label="Image preview"
        onMouseDown={stopBackdrop}
      >
        <header>
          <span className="image-lightbox-position">
            {images.length > 1 ? `${activeIndex + 1} / ${images.length}` : "Image"}
          </span>
          <div className="image-lightbox-actions">
            <button
              type="button"
              aria-label="Zoom out"
              disabled={zoom <= 0.5}
              onClick={() => setZoom((current) => Math.max(0.5, current - 0.5))}
            >
              −
            </button>
            <output aria-label="Image zoom">{Math.round(zoom * 100)}%</output>
            <button
              type="button"
              aria-label="Zoom in"
              disabled={zoom >= 4}
              onClick={() => setZoom((current) => Math.min(4, current + 0.5))}
            >
              +
            </button>
            <button
              type="button"
              disabled={saveState === "saving"}
              onClick={() => void save()}
            >
              {saveState === "saving" ? "Saving…" : "Save"}
            </button>
            <button
              ref={closeButton}
              type="button"
              aria-label="Close image preview"
              onClick={onClose}
            >
              ×
            </button>
          </div>
        </header>
        <div
          className="image-lightbox-canvas"
          data-navigation={images.length > 1}
        >
          {images.length > 1 ? (
            <button
              type="button"
              className="image-lightbox-previous"
              aria-label="Previous image"
              onClick={() =>
                onSelect((activeIndex - 1 + images.length) % images.length)
              }
            >
              ‹
            </button>
          ) : null}
          <div className="image-lightbox-viewport">
            <div
              className="image-lightbox-image-stage"
              style={{
                width: `${zoom * 100}%`,
                height: `${zoom * 100}%`,
              }}
            >
              <img
                key={image.key}
                src={image.source}
                alt={image.alt}
                draggable={false}
              />
            </div>
          </div>
          {images.length > 1 ? (
            <button
              type="button"
              className="image-lightbox-next"
              aria-label="Next image"
              onClick={() => onSelect((activeIndex + 1) % images.length)}
            >
              ›
            </button>
          ) : null}
        </div>
        <p className="image-lightbox-status" aria-live="polite">
          {saveState === "saved"
            ? "Image saved."
            : saveState === "canceled"
              ? "Save canceled."
              : saveState === "failed"
                ? "Image could not be saved."
                : ""}
        </p>
      </section>
    </div>,
    document.body,
  );
}
