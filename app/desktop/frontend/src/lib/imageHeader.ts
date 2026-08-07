// An image's pixel size, read from the front of its own bytes.
//
// WHY, when the browser will happily work it out: it works it out one frame too late.
// An `<img>` with no width/height occupies nothing until it has decoded, so a transcript
// holding one lays out at zero height and then jumps — measured on a 400x300 attachment,
// the row went 0 -> 0 -> 256px, which in a bottom-pinned stream is 256px of everything
// below it moving while the reader is looking at it. Explicit dimensions on the element
// are the standard answer to that, and they need the size before paint.
//
// Every image here arrives as inline base64 (MULTIMODAL_IMAGE_INPUT, API.md §4.3), so the
// bytes are already in hand and the size is in the first few dozen of them. Only the
// header is decoded — enough base64 for the largest container below, not the whole
// payload, which for a photo is megabytes of work nobody asked for.
//
// Returns null when the format is unknown or the header is truncated. That is a real
// answer, not a failure: the caller falls back to letting the browser decide, which is
// exactly today's behaviour.

export interface PixelSize {
  width: number;
  height: number;
}

/** Enough base64 to cover the largest header this reads. JPEG needs the most, because
 *  its size lives past a run of variable-length segments. */
const HEADER_CHARS = 4096;

function headerBytes(base64: string): Uint8Array | null {
  // Base64 decodes in 4-char groups; a partial group throws.
  const slice = base64.slice(0, HEADER_CHARS - (HEADER_CHARS % 4));
  try {
    const binary = atob(slice);
    const bytes = new Uint8Array(binary.length);
    for (let i = 0; i < binary.length; i += 1) bytes[i] = binary.charCodeAt(i);
    return bytes;
  } catch {
    return null;
  }
}

const be32 = (b: Uint8Array, at: number) =>
  (b[at]! << 24) | (b[at + 1]! << 16) | (b[at + 2]! << 8) | b[at + 3]!;
const be16 = (b: Uint8Array, at: number) => (b[at]! << 8) | b[at + 1]!;
const le16 = (b: Uint8Array, at: number) => b[at]! | (b[at + 1]! << 8);
const le32 = (b: Uint8Array, at: number) =>
  b[at]! | (b[at + 1]! << 8) | (b[at + 2]! << 16) | (b[at + 3]! << 24);

const starts = (b: Uint8Array, at: number, signature: readonly number[]) =>
  signature.every((byte, i) => b[at + i] === byte);

function pngSize(b: Uint8Array): PixelSize | null {
  // 8-byte signature, then an IHDR chunk whose first two fields are the dimensions.
  if (!starts(b, 0, [0x89, 0x50, 0x4e, 0x47])) return null;
  if (b.length < 24) return null;
  return { width: be32(b, 16), height: be32(b, 20) };
}

function gifSize(b: Uint8Array): PixelSize | null {
  if (!starts(b, 0, [0x47, 0x49, 0x46])) return null;
  if (b.length < 10) return null;
  return { width: le16(b, 6), height: le16(b, 8) };
}

function jpegSize(b: Uint8Array): PixelSize | null {
  if (!starts(b, 0, [0xff, 0xd8])) return null;
  // Walk the marker segments to the frame header. Sizes are NOT at a fixed offset: EXIF,
  // colour profiles and thumbnails all sit in front of it at whatever length they please.
  let at = 2;
  while (at + 9 < b.length) {
    if (b[at] !== 0xff) return null;
    const marker = b[at + 1]!;
    // SOFn — every frame kind carries the same size fields. C4/C8/CC are not frames.
    if (marker >= 0xc0 && marker <= 0xcf && marker !== 0xc4 && marker !== 0xc8 && marker !== 0xcc) {
      return { height: be16(b, at + 5), width: be16(b, at + 7) };
    }
    if (marker === 0xd8 || (marker >= 0xd0 && marker <= 0xd9)) {
      at += 2; // standalone marker, no length field
      continue;
    }
    at += 2 + be16(b, at + 2);
  }
  return null;
}

function webpSize(b: Uint8Array): PixelSize | null {
  if (!starts(b, 0, [0x52, 0x49, 0x46, 0x46]) || !starts(b, 8, [0x57, 0x45, 0x42, 0x50])) {
    return null;
  }
  // Three encodings, three places to look.
  if (starts(b, 12, [0x56, 0x50, 0x38, 0x20]) && b.length >= 30) {
    // Lossy: a VP8 key frame, dimensions in the low 14 bits of two little-endian words.
    return { width: le16(b, 26) & 0x3fff, height: le16(b, 28) & 0x3fff };
  }
  if (starts(b, 12, [0x56, 0x50, 0x38, 0x4c]) && b.length >= 25) {
    // Lossless: 14 bits each, packed across four bytes, both stored one less than actual.
    const bits = le32(b, 21);
    return { width: (bits & 0x3fff) + 1, height: ((bits >> 14) & 0x3fff) + 1 };
  }
  if (starts(b, 12, [0x56, 0x50, 0x38, 0x58]) && b.length >= 30) {
    // Extended: 24-bit canvas size, also stored one less than actual.
    const w = b[24]! | (b[25]! << 8) | (b[26]! << 16);
    const h = b[27]! | (b[28]! << 8) | (b[29]! << 16);
    return { width: w + 1, height: h + 1 };
  }
  return null;
}

/**
 * The pixel size of a base64-encoded image, or null if its header does not say.
 *
 * Covers the formats a webview will render from a data URL: PNG, JPEG, GIF, WebP. AVIF is
 * absent on purpose — its size sits inside a nested ISOBMFF box tree, which is a parser
 * rather than a header read, and no adapter produces one today.
 */
export function imageSizeFromBase64(base64: string): PixelSize | null {
  const bytes = headerBytes(base64);
  if (!bytes) return null;
  const size = pngSize(bytes) ?? jpegSize(bytes) ?? gifSize(bytes) ?? webpSize(bytes);
  if (!size || size.width <= 0 || size.height <= 0) return null;
  return size;
}
