import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type MouseEventHandler,
  type ReactElement,
} from "react";
import { toast } from "sonner";
import { useT } from "@/lib/i18n";
import { IconButton, LightboxDialog } from "@/ui";
import { saveInlineImage } from "../adapters/desktopImageSave";
import { MESSAGE_CONTENT_SELECTOR } from "./messageContent";

const PREVIEW_TRIGGER_ATTR = "data-message-image-preview-trigger";
const ZOOM_STEPS = [100, 125, 150, 200, 300, 400] as const;

interface GalleryItem {
  src: string;
  alt: string;
  title?: string;
  width?: number;
  height?: number;
}

interface GalleryState {
  items: GalleryItem[];
  index: number;
}

interface FittedImageSize {
  src: string;
  width: number;
  height: number;
}

interface TriggerProps {
  "data-message-image-preview-trigger": "true";
  onClick: MouseEventHandler<HTMLButtonElement>;
}

interface Props {
  item: GalleryItem;
  titleFallback: string;
  trigger: (props: TriggerProps) => ReactElement;
}

function numericAttribute(image: HTMLImageElement, name: "width" | "height") {
  const value = image.getAttribute(name);
  if (!value) return undefined;
  const parsed = Number(value);
  return Number.isFinite(parsed) && parsed > 0 ? parsed : undefined;
}

function galleryRoot(trigger: HTMLButtonElement) {
  return trigger.closest(MESSAGE_CONTENT_SELECTOR) ?? trigger.closest(".md");
}

function collectGallery(trigger: HTMLButtonElement, fallback: GalleryItem): GalleryState {
  const root = galleryRoot(trigger);
  if (!root) return { items: [fallback], index: 0 };

  const messageRoot = root.matches(MESSAGE_CONTENT_SELECTOR) ? root : null;
  const markdownRoot = messageRoot ? null : root;
  const triggers = Array.from(
    root.querySelectorAll<HTMLButtonElement>(`button[${PREVIEW_TRIGGER_ATTR}="true"]`),
  ).filter((candidate) =>
    messageRoot
      ? candidate.closest(MESSAGE_CONTENT_SELECTOR) === messageRoot
      : candidate.closest(".md") === markdownRoot,
  );

  const items: GalleryItem[] = [];
  let index = 0;
  for (const candidate of triggers) {
    const image = candidate.querySelector<HTMLImageElement>("img");
    const src = image?.currentSrc || image?.getAttribute("src") || "";
    if (!image || !src) continue;
    if (candidate === trigger) index = items.length;
    items.push({
      src,
      alt: image.alt,
      title: image.title || undefined,
      width: numericAttribute(image, "width"),
      height: numericAttribute(image, "height"),
    });
  }
  return items.length > 0 ? { items, index } : { items: [fallback], index: 0 };
}

/** A presentation-local Codex image gallery. Each trigger owns its dialog while the
 * gallery is discovered from the nearest exact message body at open time, so streamed
 * content is current and nested delegated messages never leak into their parent. */
export function ImagePreviewGallery({ item, titleFallback, trigger }: Props) {
  const t = useT();
  const [zoomed, setZoomed] = useState(false);
  const [gallery, setGallery] = useState<GalleryState | null>(null);
  const [zoomIndex, setZoomIndex] = useState(0);
  const [fittedSize, setFittedSize] = useState<FittedImageSize | null>(null);
  const [saving, setSaving] = useState(false);
  const savingRef = useRef(false);

  const setGalleryIndex = useCallback((index: number) => {
    setZoomIndex(0);
    setFittedSize(null);
    setGallery((current) =>
      current
        ? { ...current, index: Math.max(0, Math.min(index, current.items.length - 1)) }
        : null,
    );
  }, []);

  useEffect(() => {
    if (!zoomed || !gallery) return;
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "ArrowLeft" && gallery.index > 0) {
        event.preventDefault();
        setGalleryIndex(gallery.index - 1);
      } else if (event.key === "ArrowRight" && gallery.index < gallery.items.length - 1) {
        event.preventDefault();
        setGalleryIndex(gallery.index + 1);
      }
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [gallery, setGalleryIndex, zoomed]);

  const active = gallery?.items[gallery.index] ?? item;
  const hasGallery = (gallery?.items.length ?? 0) > 1;
  const zoomPercent = ZOOM_STEPS[zoomIndex]!;
  const measuredSize = fittedSize?.src === active.src ? fittedSize : null;
  const zoomedSize =
    measuredSize && zoomPercent > 100
      ? {
          width: `${(measuredSize.width * zoomPercent) / 100}px`,
          height: `${(measuredSize.height * zoomPercent) / 100}px`,
          maxWidth: "none",
          maxHeight: "none",
        }
      : undefined;
  const closePreview = () => {
    setZoomed(false);
    setGallery(null);
    setZoomIndex(0);
    setFittedSize(null);
  };
  const saveActiveImage = async () => {
    if (savingRef.current) return;
    savingRef.current = true;
    setSaving(true);
    try {
      await saveInlineImage(active.src);
    } catch {
      toast.error(t("message.image.downloadFailed"));
    } finally {
      savingRef.current = false;
      setSaving(false);
    }
  };

  return (
    <LightboxDialog
      open={zoomed}
      onOpenChange={(open) => {
        setZoomed(open);
        if (!open) {
          setGallery(null);
          setZoomIndex(0);
          setFittedSize(null);
        }
      }}
      title={active.alt || titleFallback}
      className="h-[100dvh] w-screen max-h-none max-w-none overflow-hidden rounded-none bg-media-preview p-0 shadow-none"
      trigger={trigger({
        "data-message-image-preview-trigger": "true",
        onClick: (event) => {
          setZoomIndex(0);
          setFittedSize(null);
          setGallery(collectGallery(event.currentTarget, item));
        },
      })}
    >
      <div className="relative flex size-full flex-col" data-image-zoom={zoomPercent}>
        <div className="absolute top-3 right-3 z-1 flex items-center gap-1">
          <IconButton
            icon="download"
            size="lg"
            title={t("message.image.download")}
            aria-busy={saving}
            disabled={saving}
            onClick={() => void saveActiveImage()}
            className="size-10 bg-media-scrim text-on-media hover:bg-media-scrim"
          />
          <IconButton
            icon="x"
            size="lg"
            title={t("message.image.close")}
            onClick={closePreview}
            className="size-10 bg-media-scrim text-on-media hover:bg-media-scrim"
          />
        </div>
        {hasGallery && (
          <>
            <IconButton
              icon="chevron-left"
              size="lg"
              title={t("message.image.previous")}
              disabled={gallery!.index === 0}
              onClick={(event) => {
                event.stopPropagation();
                setGalleryIndex(gallery!.index - 1);
              }}
              className="absolute top-1/2 left-3 z-1 size-10 -translate-y-1/2 bg-media-scrim text-on-media hover:bg-media-scrim"
            />
            <IconButton
              icon="chevron-right"
              size="lg"
              title={t("message.image.next")}
              disabled={gallery!.index === gallery!.items.length - 1}
              onClick={(event) => {
                event.stopPropagation();
                setGalleryIndex(gallery!.index + 1);
              }}
              className="absolute top-1/2 right-3 z-1 size-10 -translate-y-1/2 bg-media-scrim text-on-media hover:bg-media-scrim"
            />
          </>
        )}
        <div className="min-h-0 flex-1 overflow-auto p-4 pt-12 pb-16">
          <div className="grid min-h-full min-w-full place-items-center">
            <img
              src={active.src}
              alt={active.alt}
              title={active.title}
              width={active.width}
              height={active.height}
              style={zoomedSize}
              onLoad={(event) => {
                if (zoomPercent !== 100) return;
                const { width, height } = event.currentTarget.getBoundingClientRect();
                if (width > 0 && height > 0) setFittedSize({ src: active.src, width, height });
              }}
              className="block max-h-[calc(100dvh-8rem)] max-w-[calc(100vw-2rem)] rounded-lg object-contain media-edge"
            />
          </div>
        </div>
        <div className="absolute bottom-3 left-1/2 z-1 flex -translate-x-1/2 items-center gap-1 rounded-full bg-media-scrim p-1 text-on-media shadow-[var(--shadow-floating)]">
          <IconButton
            icon="zoom-out"
            size="lg"
            title={t("message.image.zoomOut")}
            disabled={zoomIndex === 0}
            onClick={() => setZoomIndex((current) => Math.max(0, current - 1))}
            className="size-10 text-on-media hover:bg-media-scrim"
          />
          <span className="min-w-14 px-1 text-center font-mono text-ui-sm tabular-nums">
            {zoomPercent}%
          </span>
          <IconButton
            icon="zoom-in"
            size="lg"
            title={t("message.image.zoomIn")}
            disabled={zoomIndex === ZOOM_STEPS.length - 1}
            onClick={() => setZoomIndex((current) => Math.min(ZOOM_STEPS.length - 1, current + 1))}
            className="size-10 text-on-media hover:bg-media-scrim"
          />
        </div>
      </div>
    </LightboxDialog>
  );
}
