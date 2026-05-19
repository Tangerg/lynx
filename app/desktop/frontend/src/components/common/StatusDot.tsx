import { cn } from "./cn";

// Small colored dot. The visual tone is controlled by the parent's CSS scope
// (e.g. .session-sub .status-dot.running, .chat-tab .tab-dot.running). The
// `as` prop lets callers pick between status-dot / tab-dot / tool-status etc.
type Props = {
  tone?: string;
  as?: string;
  className?: string;
};

export function StatusDot({ tone, as = "status-dot", className }: Props) {
  return <span className={cn(as, tone, className)} />;
}
