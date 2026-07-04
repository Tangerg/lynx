import { Divider, Icon } from "@/ui";

// Checkpoint — a "milestone reached" marker between message chunks.
// Thin wrapper around <Divider> with the canonical check glyph.
export function Checkpoint({ text }: { text: string }) {
  return (
    <Divider icon={<Icon name="check" size={11} strokeWidth={3} />} intent="accent">
      {text}
    </Divider>
  );
}
