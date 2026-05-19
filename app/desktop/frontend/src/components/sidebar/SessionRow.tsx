import { Icon, StatusDot } from "@/components/common";
import type { SidebarSession } from "./types";

type Props = {
  session: SidebarSession;
  active: boolean;
  onSelect: (id: string) => void;
};

export function SessionRow({ session, active, onSelect }: Props) {
  const subText =
    session.status === "running" ? "Running" :
    session.status === "waiting" ? "Needs input" :
    session.model;

  return (
    <div
      className={`session-row ${active ? "active" : ""}`}
      onClick={() => onSelect(session.id)}
    >
      <div className="session-icon"><Icon name="chat" size={14} /></div>
      <div className="session-body">
        <div className="session-title">{session.title}</div>
        <div className="session-sub">
          <StatusDot tone={session.status} />
          <span>{subText}</span>
        </div>
      </div>
      <div className="session-time">{session.time}</div>
    </div>
  );
}
