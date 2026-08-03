import { useT } from "@/lib/i18n";
import { AgentWindowControls } from "@/ui/agent";
import { windowCommands } from "../adapters/windowCommands";

/** Binds the shell's window controls to the desktop host. The marks render even
 *  where no window answers: this is the one piece of chrome whose absence would
 *  be read as a broken window rather than a missing feature. */
export function WindowControls() {
  const t = useT();
  return (
    <AgentWindowControls
      onClose={windowCommands.close}
      onMinimise={windowCommands.minimise}
      onToggleMaximise={windowCommands.toggleMaximise}
      closeLabel={t("window.action.close")}
      minimiseLabel={t("window.action.minimise")}
      maximiseLabel={t("window.action.maximise")}
    />
  );
}
