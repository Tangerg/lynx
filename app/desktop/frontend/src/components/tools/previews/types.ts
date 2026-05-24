// Generic shapes that the inline tool previews consume. They're intentionally
// not tied to AG-UI — any data source can map onto these.

export type TermLine = {
  kind: "prompt" | "cmd" | "out" | "err" | "warn" | "mute" | "ok";
  text: string;
};

export type DiffRow =
  | { type: "hunk"; text: string }
  | { type: "ctx"; l: number; r: number; code: string }
  | { type: "add"; r: number; code: string }
  | { type: "del"; l: number; code: string };

export type GrepMatch = {
  path: string;
  match: string;
};

export type FileLine = {
  ln: string; // line number or marker like "···"
  code: string; // already-rendered HTML
  muted?: boolean;
};
