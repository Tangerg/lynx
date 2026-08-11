// Published actions for opening the session finder from another bounded context.
import { sessionSearchLauncher } from "../application/ports/sessionSearchLauncher";

export function openSessionSearch(): void {
  sessionSearchLauncher().open();
}
