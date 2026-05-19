import { Icon, type IconName } from "@/components/common";

/**
 * One rail button descriptor. `useBadge` is an optional hook the rail
 * invokes per-button so plugin tabs can subscribe to their own data
 * (e.g. `useFilesChanged().length`) without the parent component knowing.
 */
export type RailBtn = {
  id: string;
  icon: IconName;
  label: string;
  useBadge?: () => number | undefined;
};

type Props = {
  open: boolean;
  tab: string;
  buttons: RailBtn[];
  onClick: (id: string) => void;
  onClose: () => void;
};

export function InspectorRail({ open, tab, buttons, onClick, onClose }: Props) {
  return (
    <div className="insp-rail">
      {buttons.map((b) => (
        <RailButton
          key={b.id}
          btn={b}
          active={open && tab === b.id}
          onClick={() => onClick(b.id)}
        />
      ))}
      <div className="insp-rail-spacer" />
      <div className="insp-rail-bottom">
        {open && (
          <button className="insp-rail-btn" onClick={onClose} title="Collapse panel (⌘\\)">
            <Icon name="panel" size={15} />
          </button>
        )}
      </div>
    </div>
  );
}

// Each button is its own component so the optional badge hook obeys
// Rules-of-Hooks (same hook call site every render of *that* component).
function RailButton({ btn, active, onClick }: { btn: RailBtn; active: boolean; onClick: () => void }) {
  const badge = btn.useBadge ? btn.useBadge() : undefined;
  return (
    <button
      className={`insp-rail-btn ${active ? "active" : ""}`}
      onClick={onClick}
      title={btn.label}
    >
      <Icon name={btn.icon} size={15} />
      {badge != null && badge > 0 && <span className="rail-badge">{badge}</span>}
    </button>
  );
}
