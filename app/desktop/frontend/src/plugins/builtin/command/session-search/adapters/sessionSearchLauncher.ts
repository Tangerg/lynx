import { useSessionSearchStore } from "../application/sessionSearchState";
import { configureSessionSearchLauncherPort } from "../application/ports/sessionSearchLauncher";

// A controlled Base UI dialog has no trigger node to return to. Keep that
// browser-only concern in the adapter: the application state is only whether
// the session finder is open.
let returnFocus: HTMLElement | null = null;

function captureReturnFocus(): void {
  returnFocus = document.activeElement instanceof HTMLElement ? document.activeElement : null;
}

export function sessionSearchReturnFocus(): HTMLElement | null {
  return returnFocus?.isConnected ? returnFocus : null;
}

function open(): void {
  captureReturnFocus();
  useSessionSearchStore.getState().show();
}

function toggle(): void {
  const state = useSessionSearchStore.getState();
  if (state.open) {
    state.setOpen(false);
    return;
  }
  captureReturnFocus();
  state.show();
}

export function installSessionSearchLauncher(): () => void {
  return configureSessionSearchLauncherPort({
    open,
    toggle,
  });
}
