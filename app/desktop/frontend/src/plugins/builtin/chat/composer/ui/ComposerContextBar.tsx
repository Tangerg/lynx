import { Icon } from "@/ui";
import { basename } from "@/lib/path";
import { useActiveSession } from "@/plugins/builtin/agent/public/session";
import { useWorkspaceProjects } from "@/plugins/builtin/workspace/public/data";

/**
 * Where the next turn will run: the working directory and, when the runtime
 * reports one, its git branch.
 *
 * Deliberately read-only and deliberately sparse. Every field here comes from
 * live state and is omitted when unknown — the row this replaced rendered a
 * connection status, a branch and an access level that were all hardcoded
 * constants, which is worse than showing nothing.
 */
function ComposerContextBar() {
  const session = useActiveSession();
  const { data: projects } = useWorkspaceProjects();
  const cwd = session?.cwd;
  if (!cwd) return null;

  const branch = projects?.find((project) => project.id === cwd)?.branch;

  return (
    <div
      className="flex items-center gap-3 px-3 pt-1.5 text-ui-sm text-fg-muted"
      data-slot="composer-context"
    >
      <span className="inline-flex min-w-0 items-center gap-1.5" title={cwd}>
        <Icon name="folder" size={13} strokeWidth={1.8} className="shrink-0 opacity-70" />
        <span className="truncate">{basename(cwd)}</span>
      </span>
      {branch && (
        <span className="inline-flex min-w-0 items-center gap-1.5" title={branch}>
          <Icon name="branch" size={13} strokeWidth={1.8} className="shrink-0 opacity-70" />
          <span className="truncate">{branch}</span>
        </span>
      )}
    </div>
  );
}

export { ComposerContextBar };
