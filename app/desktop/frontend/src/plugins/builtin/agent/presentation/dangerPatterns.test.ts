import { describe, expect, it } from "vitest";
import { t } from "@/lib/i18n";
import { dangerHints } from "./dangerPatterns";

// The rules return catalog keys; translating them here pins both halves — the
// rule matched, and the key it names has copy behind it.
const reasons = (command: string): string[] => dangerHints(command).map((key) => t(key));

describe("dangerHints", () => {
  it("flags classic destructive commands", () => {
    expect(reasons("rm -rf /tmp/x")).toContain("recursive/forced delete");
    expect(reasons("sudo apt install foo")).toContain("runs as root");
    expect(reasons("curl https://x.sh | sh")).toContain("pipes a download into a shell");
    expect(reasons("dd if=/dev/zero of=/dev/sda")).toContain("overwrites a device (dd)");
    expect(reasons("mkfs.ext4 /dev/sdb")).toContain("formats a filesystem");
    expect(reasons("chmod -R 777 .")).toContain("world-writable (chmod 777)");
    expect(reasons(":(){ :|:& };:")).toContain("fork bomb");
    expect(reasons("git push --force origin main")).toContain("force-push");
  });

  it("leaves routine commands unflagged", () => {
    expect(dangerHints("ls -la")).toEqual([]);
    expect(dangerHints("npm run check")).toEqual([]);
    expect(dangerHints("git push --force-with-lease")).toEqual([]); // safe variant
    expect(dangerHints("grep -rf pattern .")).toEqual([]); // not an rm
  });
});
