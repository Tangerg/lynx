// Built-in plugin: composer-footer chips (working directory, execution
// mode, git branch). Each chip is its own registration so user plugins
// can hide/replace individual ones.
//
// Values are still hardcoded strings — the original UI had them as
// constant props from AgentClientPage. Wiring them to real state (active
// project, current branch via git, etc.) is a follow-up that doesn't
// touch the registration API.

import { Icon, type IconName } from "@/components/common";
import { definePlugin } from "@/plugins/sdk";

function Chip({
  icon, title, children,
}: { icon: IconName; title: string; children: React.ReactNode }) {
  return (
    <button className="cf-chip" title={title}>
      <Icon name={icon} size={11} />
      <span>{children}</span>
      <Icon name="more" size={10} />
    </button>
  );
}

const Project    = () => <Chip icon="folder" title="Working directory">fern-api</Chip>;
const ExecMode   = () => <Chip icon="shield" title="Execution mode">Workspace · Auto</Chip>;
const GitBranch  = () => <Chip icon="branch" title="Git branch">feat/result-type</Chip>;

export default definePlugin({
  name: "lyra.builtin.composer-chips",
  version: "1.0.0",
  setup({ host }) {
    host.composer.registerStatus({ id: "project",   order: 0, component: Project });
    host.composer.registerStatus({ id: "exec-mode", order: 1, component: ExecMode });
    host.composer.registerStatus({ id: "git-branch", order: 2, component: GitBranch });
  },
});
