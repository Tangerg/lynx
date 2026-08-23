import type { ContentBlock } from "@lyra/runtime-contract";
import { useMemo } from "react";

import { ImageGallery, type GalleryImage } from "./ImageGallery";
import { MarkdownContent } from "./MarkdownContent";

interface NarrativeContentProps {
  content?: ContentBlock[];
  highlight: string;
}

type ContentSection =
  | { type: "text"; key: string; text: string }
  | { type: "images"; key: string; images: GalleryImage[] };

const imageMIMETypes = new Set([
  "image/avif",
  "image/gif",
  "image/jpeg",
  "image/png",
  "image/svg+xml",
  "image/webp",
]);
const maxInlineImageBytes = 32 << 20;
const maxEncodedImageLength = Math.ceil(maxInlineImageBytes / 3) * 4;
const base64Pattern = /^(?:[A-Za-z0-9+/]{4})*(?:[A-Za-z0-9+/]{2}==|[A-Za-z0-9+/]{3}=)?$/;
const emptyContent: readonly ContentBlock[] = [];

export function NarrativeContent({
  content,
  highlight,
}: NarrativeContentProps) {
  const sections = useMemo(
    () => sectionsOf(content ?? emptyContent),
    [content],
  );
  return (
    <div className="message-content">
      {sections.map((section) =>
        section.type === "text" ? (
          <MarkdownContent
            key={section.key}
            source={section.text}
            highlight={highlight}
          />
        ) : (
          <ImageGallery key={section.key} images={section.images} />
        ),
      )}
    </div>
  );
}

function sectionsOf(content: readonly ContentBlock[]): ContentSection[] {
  const sections: ContentSection[] = [];
  let images: GalleryImage[] = [];

  const flushImages = () => {
    if (images.length === 0) return;
    const first = images[0];
    sections.push({
      type: "images",
      key: `images:${first.key}`,
      images,
    });
    images = [];
  };

  content.forEach((block, index) => {
    if (block.type === "image") {
      const image = imageOf(block, index);
      if (image !== undefined) images.push(image);
      return;
    }
    flushImages();
    if (block.type === "text" && block.text) {
      sections.push({ type: "text", key: `text:${index}`, text: block.text });
    }
  });
  flushImages();
  return sections;
}

function imageOf(block: ContentBlock, index: number): GalleryImage | undefined {
  const mime = block.mime?.toLocaleLowerCase();
  const data = block.data;
  if (
    mime === undefined ||
    !imageMIMETypes.has(mime) ||
    data === undefined ||
    data.length === 0 ||
    data.length > maxEncodedImageLength ||
    !base64Pattern.test(data)
  ) {
    return undefined;
  }
  return {
    key: `${index}:${mime}:${data.length}`,
    source: `data:${mime};base64,${data}`,
    alt: `Attached image ${index + 1}`,
  };
}
