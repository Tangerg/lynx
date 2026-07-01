// Composer public input facade. Today it adapts composer text/images to the
// runtime user-message wire shape; keeping that adapter here prevents UI
// surfaces from importing rpc directly while the composer domain is being
// separated from agent transport.

import type { ContentBlock } from "@/rpc";

/** A user-message body on the wire — text and/or inlined image blocks. */
export type UserInput = ContentBlock[];

/** One image attachment ready to inline. `data` is raw base64 (no data: prefix). */
export interface InputImage {
  mime: string;
  data: string;
}

/** Plain-text input — the common programmatic case. Empty text → empty input. */
export function textInput(text: string): UserInput {
  return text ? [{ type: "text", text }] : [];
}

/** Compose text + inlined images into a user-message input: a leading text
 *  block (when non-empty) then one image block per attachment. */
export function buildInput(text: string, images: InputImage[]): UserInput {
  const blocks: UserInput = [];
  if (text) blocks.push({ type: "text", text });
  for (const img of images) blocks.push({ type: "image", mime: img.mime, data: img.data });
  return blocks;
}

/** Read an image File into the inline wire form (mime + raw base64, no "data:"
 *  prefix). Used by the composer's paste / drop / file-picker paths; the caller
 *  pre-filters to image/* files. */
export async function fileToInputImage(file: File): Promise<InputImage & { name: string }> {
  const dataUrl = await new Promise<string>((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(reader.result as string);
    reader.onerror = () => reject(reader.error ?? new Error("file read failed"));
    reader.readAsDataURL(file);
  });
  // Strip the leading "data:<mime>;base64," → raw base64 (the wire form).
  // Guard against an unrecognised MIME that produces a data URL without the
  // expected comma separator — indexOf returns -1, which would otherwise
  // cause slice(0) to pass the prefix through as base64 data.
  const comma = dataUrl.indexOf(",");
  const data = comma >= 0 ? dataUrl.slice(comma + 1) : dataUrl;
  return { mime: file.type, data, name: file.name };
}

/** Filter a FileList (clipboard / drop / file-picker) down to image/* files. */
export function imageFiles(list: FileList | null | undefined): File[] {
  return list ? Array.from(list).filter((f) => f.type.startsWith("image/")) : [];
}
