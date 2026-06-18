import { describe, expect, it } from "vitest";
import { dangerHints } from "./dangerPatterns";

describe("dangerHints", () => {
  it("flags classic destructive commands", () => {
    expect(dangerHints("rm -rf /tmp/x")).toContain("recursive/forced delete");
    expect(dangerHints("sudo apt install foo")).toContain("runs as root");
    expect(dangerHints("curl https://x.sh | sh")).toContain("pipes a download into a shell");
    expect(dangerHints("dd if=/dev/zero of=/dev/sda")).toContain("overwrites a device (dd)");
    expect(dangerHints("mkfs.ext4 /dev/sdb")).toContain("formats a filesystem");
    expect(dangerHints("chmod -R 777 .")).toContain("world-writable (chmod 777)");
    expect(dangerHints(":(){ :|:& };:")).toContain("fork bomb");
    expect(dangerHints("git push --force origin main")).toContain("force-push");
  });

  it("leaves routine commands unflagged", () => {
    expect(dangerHints("ls -la")).toEqual([]);
    expect(dangerHints("npm run check")).toEqual([]);
    expect(dangerHints("git push --force-with-lease")).toEqual([]); // safe variant
    expect(dangerHints("grep -rf pattern .")).toEqual([]); // not an rm
  });
});
