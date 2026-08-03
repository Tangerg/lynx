import { getContainer } from "@/main/container";

/** The three window commands, as the rest of this context sees them. Each is a
 *  no-op outside a Wails window — a browser tab has no window to minimise, and a
 *  dead control is not worth an error path. */
export const windowCommands = {
  close: () => getContainer().desktop.closeWindow(),
  minimise: () => getContainer().desktop.minimiseWindow(),
  toggleMaximise: () => getContainer().desktop.toggleMaximiseWindow(),
};
