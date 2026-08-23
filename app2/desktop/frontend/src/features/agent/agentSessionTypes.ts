export interface LiveToolOutput {
  text: string;
  truncated: boolean;
}

export type SessionActivityView =
  "overview" | "timeline" | "terminal" | "summary";
