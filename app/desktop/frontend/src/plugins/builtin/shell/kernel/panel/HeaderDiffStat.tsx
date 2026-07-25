import { Button } from "@/ui";
import { cn } from "@/lib/utils";
import { useT } from "@/lib/i18n";
import { useActiveSessionCwd } from "@/plugins/builtin/agent/public/session";
import { openWorkspaceViewBeside } from "@/plugins/builtin/workspace/public/navigation";
import {
  useWorkspaceCapability,
  useWorkspaceFileChanges,
} from "@/plugins/builtin/workspace/public/data";

/**
 * Working-tree churn at a glance — `+added −removed`, opening the diff view.
 *
 * Renders nothing rather than zeros when there is no churn, when the git
 * capability is off, or while the query is in flight: a header readout that is
 * always present but usually says "+0 −0" trains the eye to skip it.
 */
export function HeaderDiffStat({ className }: { className?: string }) {
  const t = useT();
  const gitEnabled = useWorkspaceCapability("git");
  const cwd = useActiveSessionCwd();
  const { data: files } = useWorkspaceFileChanges(gitEnabled ? { cwd } : undefined);

  const totals = (files ?? []).reduce(
    (sum, file) => ({
      added: sum.added + (file.added ?? 0),
      removed: sum.removed + (file.removed ?? 0),
    }),
    { added: 0, removed: 0 },
  );
  if (totals.added === 0 && totals.removed === 0) return null;

  return (
    <Button
      size="sm"
      press={false}
      data-chrome-focus=""
      aria-label={t("workspace.view.title.diff")}
      onClick={() => openWorkspaceViewBeside("diff")}
      className={cn("gap-1.5 px-1.5 font-mono text-ui-sm tabular-nums", className)}
    >
      <span className="text-success">+{totals.added}</span>
      <span className="text-negative">−{totals.removed}</span>
    </Button>
  );
}
