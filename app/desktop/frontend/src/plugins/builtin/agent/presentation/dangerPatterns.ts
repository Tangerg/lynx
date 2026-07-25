// Client-side danger heuristics for a shell command awaiting approval. This is
// an extra presentation warning independent of the backend's risk field: a miss
// just means no banner, while a false hit only adds a banner.

interface DangerRule {
  re: RegExp;
  /** Catalog key for the short reason the banner shows (joined by " · ").
   *  A key, not the words: this ring maps a model into a view model and has no
   *  business holding one locale's copy — nine English reasons here meant seven
   *  languages read the approval banner in English. */
  labelKey: string;
}

const RULES: readonly DangerRule[] = [
  { re: /\brm\s+-[a-z]*[rf]/i, labelKey: "danger.recursiveDelete" },
  { re: /\bsudo\b|\bdoas\b/i, labelKey: "danger.runsAsRoot" },
  {
    re: /\b(?:curl|wget)\b[^\n|]*\|\s*(?:sudo\s+)?(?:ba|z|fi)?sh\b/i,
    labelKey: "danger.pipeToShell",
  },
  { re: /\bdd\b[^\n]*\bof=/i, labelKey: "danger.overwritesDevice" },
  { re: /\bmkfs\b/i, labelKey: "danger.formatsFilesystem" },
  { re: /\bchmod\s+(?:-R\s+)?0?777\b/i, labelKey: "danger.worldWritable" },
  { re: /\{\s*:\s*\|\s*:\s*&\s*\}/, labelKey: "danger.forkBomb" },
  { re: />\s*\/dev\/(?:sd|nvme|disk|hd)/i, labelKey: "danger.rawDiskWrite" },
  {
    re: /\bgit\b[^\n]*\bpush\b[^\n]*(?:-f\b|--force(?!-with-lease))/i,
    labelKey: "danger.forcePush",
  },
];

/** Catalog keys for why the command looks destructive, or [] when it looks
 *  routine. The caller translates. */
export function dangerHints(command: string): string[] {
  const hits: string[] = [];
  for (const { re, labelKey } of RULES) if (re.test(command)) hits.push(labelKey);
  return hits;
}
