import { useState } from "react";
import { Icon, PillButton } from "@/components/common";

export function ApprovalCard({ what, cmd, reason }: { what: string; cmd: string; reason: string }) {
  const [state, setState] = useState<"pending" | "approved" | "skipped">("pending");

  if (state === "approved") {
    return (
      <div className="checkpoint">
        <div className="ico"><Icon name="check" size={11} strokeWidth={3} /></div>
        <span>Approved · running command</span>
      </div>
    );
  }
  if (state === "skipped") {
    return (
      <div className="checkpoint">
        <div className="ico" style={{ color: "var(--color-text-faint)" }}><Icon name="x" size={11} /></div>
        <span style={{ color: "var(--color-text-faint)" }}>Skipped</span>
      </div>
    );
  }
  return (
    <div className="approval-card">
      <div className="head">
        <Icon name="shield" size={12} />Approval required
      </div>
      <div className="what">{what}</div>
      <code className="cmd">$ {cmd}</code>
      <div className="reason">{reason}</div>
      <div className="actions">
        <PillButton variant="accent" style={{ height: 30, fontSize: 11 }} onClick={() => setState("approved")}>
          Approve
        </PillButton>
        <PillButton style={{ height: 30, fontSize: 11 }} onClick={() => setState("skipped")}>
          Skip
        </PillButton>
        <label className="always">
          <input type="checkbox" />
          Always allow pnpm test
        </label>
      </div>
    </div>
  );
}
