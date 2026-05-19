import { Icon } from "@/components/common";

export function Checkpoint({ text }: { text: string }) {
  return (
    <div className="checkpoint">
      <div className="ico"><Icon name="check" size={11} strokeWidth={3} /></div>
      <span>{text}</span>
    </div>
  );
}
