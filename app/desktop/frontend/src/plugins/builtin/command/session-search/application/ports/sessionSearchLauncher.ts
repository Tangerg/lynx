import { createSingletonPort } from "@/lib/ports/singletonPort";

/**
 * Opens the session finder from any driving adapter and identifies where a
 * controlled dialog should return focus. The browser mechanism stays behind
 * the port; consumers only coordinate the use case.
 */
export interface SessionSearchLauncherPort {
  open(): void;
  toggle(): void;
}

const port = createSingletonPort<SessionSearchLauncherPort>(
  "Session search launcher port is not configured",
);

export const configureSessionSearchLauncherPort = port.configure;
export const sessionSearchLauncher = port.get;
