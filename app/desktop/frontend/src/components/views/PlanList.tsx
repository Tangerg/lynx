import { Icon, PillButton } from "@/components/common";
import type { PlanItem } from "@/protocol/agui/viewState";

export function PlanList({ plan }: { plan: PlanItem[] }) {
  return (
    <div style={{ padding: "14px 18px" }}>
      <div style={{
        fontSize: 10.5, fontWeight: 700, letterSpacing: "0.14em",
        textTransform: "uppercase", color: "var(--color-text-faint)", marginBottom: 12,
      }}>
        Task plan
      </div>
      {plan.map((p) => (
        <div key={p.id} className={`plan-item ${p.status}`} style={{ padding: "8px 0" }}>
          <div className="check">
            {p.status === "done" && <Icon name="check" size={12} strokeWidth={3} />}
          </div>
          <div>{p.text}</div>
        </div>
      ))}
      <ApprovalNote />
    </div>
  );
}

function ApprovalNote() {
  return (
    <div style={{
      marginTop: 16, padding: "12px 14px",
      background: "var(--color-surface)", borderRadius: 8,
      fontSize: 12, color: "var(--color-text-muted)", lineHeight: 1.5,
    }}>
      <div style={{ display: "flex", alignItems: "center", gap: 8, marginBottom: 6 }}>
        <Icon name="shield" size={13} style={{ color: "var(--color-warning)" }} />
        <span style={{
          fontWeight: 700, color: "var(--color-text)", fontSize: 11.5,
          letterSpacing: "0.04em", textTransform: "uppercase",
        }}>
          Approval required
        </span>
      </div>
      Agent will run{" "}
      <code style={{
        fontFamily: "var(--font-mono)", background: "var(--color-surface-2)",
        padding: "1px 5px", borderRadius: 3, color: "var(--color-text)",
      }}>
        pnpm test --filter=auth
      </code>{" "}
      after typecheck passes.
      <div style={{ display: "flex", gap: 6, marginTop: 10 }}>
        <PillButton variant="accent" size="sm">Approve</PillButton>
        <PillButton size="sm">Skip</PillButton>
      </div>
    </div>
  );
}
