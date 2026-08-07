import { describe, expect, it } from "vitest";
import { imageSizeFromBase64 } from "./imageHeader";

const toBase64 = (bytes: number[]) => btoa(String.fromCharCode(...bytes));

/** A PNG signature plus an IHDR chunk carrying the size. */
function png(width: number, height: number): string {
  const be = (n: number) => [(n >>> 24) & 0xff, (n >>> 16) & 0xff, (n >>> 8) & 0xff, n & 0xff];
  return toBase64([
    0x89,
    0x50,
    0x4e,
    0x47,
    0x0d,
    0x0a,
    0x1a,
    0x0a,
    ...be(13),
    0x49,
    0x48,
    0x44,
    0x52,
    ...be(width),
    ...be(height),
    8,
    6,
    0,
    0,
    0,
  ]);
}

/** SOI, then `padding` bytes of segments the reader has to walk past, then a frame. */
function jpeg(width: number, height: number, padding = 0): string {
  const segment =
    padding > 0
      ? [
          0xff,
          0xe1,
          ((padding + 2) >> 8) & 0xff,
          (padding + 2) & 0xff,
          ...Array.from({ length: padding }, () => 0x00),
        ]
      : [];
  return toBase64([
    0xff,
    0xd8,
    ...segment,
    0xff,
    0xc0,
    0x00,
    0x11,
    0x08,
    (height >> 8) & 0xff,
    height & 0xff,
    (width >> 8) & 0xff,
    width & 0xff,
    3,
    1,
    0x22,
    0,
    2,
    0x11,
    1,
    3,
    0x11,
    1,
  ]);
}

describe("imageSizeFromBase64", () => {
  it("reads a PNG's IHDR", () => {
    expect(imageSizeFromBase64(png(400, 300))).toEqual({ width: 400, height: 300 });
  });

  it("reads a GIF's logical screen descriptor, which is little-endian", () => {
    const gif = toBase64([0x47, 0x49, 0x46, 0x38, 0x39, 0x61, 0x90, 0x01, 0x2c, 0x01, 0, 0]);
    expect(imageSizeFromBase64(gif)).toEqual({ width: 400, height: 300 });
  });

  it("reads a JPEG frame header", () => {
    expect(imageSizeFromBase64(jpeg(1920, 1080))).toEqual({ width: 1920, height: 1080 });
  });

  it("walks past variable-length segments to reach the JPEG frame", () => {
    // The whole point of scanning rather than indexing: a camera JPEG puts EXIF, and
    // often a thumbnail, in front of the size.
    expect(imageSizeFromBase64(jpeg(1920, 1080, 600))).toEqual({ width: 1920, height: 1080 });
  });

  it("reads lossy WebP", () => {
    const b: number[] = Array.from({ length: 30 }, () => 0);
    [0x52, 0x49, 0x46, 0x46].forEach((v, i) => (b[i] = v));
    [0x57, 0x45, 0x42, 0x50].forEach((v, i) => (b[8 + i] = v));
    [0x56, 0x50, 0x38, 0x20].forEach((v, i) => (b[12 + i] = v));
    b[26] = 400 & 0xff;
    b[27] = (400 >> 8) & 0x3f;
    b[28] = 300 & 0xff;
    b[29] = (300 >> 8) & 0x3f;
    expect(imageSizeFromBase64(toBase64(b))).toEqual({ width: 400, height: 300 });
  });

  it("returns null rather than guessing", () => {
    expect(imageSizeFromBase64("")).toBeNull();
    expect(imageSizeFromBase64(toBase64([1, 2, 3, 4, 5, 6, 7, 8]))).toBeNull();
    // A PNG signature with the IHDR cut off — a truncated header is not a size.
    expect(
      imageSizeFromBase64(toBase64([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a])),
    ).toBeNull();
    expect(imageSizeFromBase64("not%%base64%%")).toBeNull();
  });

  it("rejects a zero dimension, which reserves nothing", () => {
    expect(imageSizeFromBase64(png(0, 300))).toBeNull();
  });
});
