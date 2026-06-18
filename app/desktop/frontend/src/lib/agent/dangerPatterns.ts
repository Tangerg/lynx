// Client-side danger heuristics for a shell command awaiting approval — a
// last-line "are you sure?" banner INDEPENDENT of the backend's risk field, so
// a destructive command is flagged even when the runtime reports no risk.
// Borrowed from Kimi code's DANGER_PATTERNS. Intentionally conservative: a
// miss just means no banner (the approval prompt still shows), a false hit only
// adds a banner — neither blocks anything.

interface DangerRule {
  re: RegExp;
  /** Short human reason, shown in the banner (joined by " · "). */
  label: string;
}

const RULES: readonly DangerRule[] = [
  { re: /\brm\s+-[a-z]*[rf]/i, label: "recursive/forced delete" },
  { re: /\bsudo\b|\bdoas\b/i, label: "runs as root" },
  {
    re: /\b(?:curl|wget)\b[^\n|]*\|\s*(?:sudo\s+)?(?:ba|z|fi)?sh\b/i,
    label: "pipes a download into a shell",
  },
  { re: /\bdd\b[^\n]*\bof=/i, label: "overwrites a device (dd)" },
  { re: /\bmkfs\b/i, label: "formats a filesystem" },
  { re: /\bchmod\s+(?:-R\s+)?0?777\b/i, label: "world-writable (chmod 777)" },
  { re: /\{\s*:\s*\|\s*:\s*&\s*\}/, label: "fork bomb" },
  { re: />\s*\/dev\/(?:sd|nvme|disk|hd)/i, label: "writes to a raw disk" },
  { re: /\bgit\b[^\n]*\bpush\b[^\n]*(?:-f\b|--force(?!-with-lease))/i, label: "force-push" },
];

/** Human reasons the command looks destructive, or [] when it looks routine. */
export function dangerHints(command: string): string[] {
  const hits: string[] = [];
  for (const { re, label } of RULES) if (re.test(command)) hits.push(label);
  return hits;
}
