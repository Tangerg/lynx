import { useEffect, useState } from "react";
import { getContainer } from "@/main/container";

/** The three window commands, as the rest of this context sees them. Each is a
 *  no-op outside a Wails window — a browser tab has no window to minimise, and a
 *  dead control is not worth an error path. */
export const windowCommands = {
  close: () => getContainer().desktop.closeWindow(),
  minimise: () => getContainer().desktop.minimiseWindow(),
  toggleMaximise: () => getContainer().desktop.toggleMaximiseWindow(),
};

/**
 * Whether the window currently fills the screen.
 *
 * Polled from the host on every resize rather than tracked from our own toggle:
 * the window is also zoomed by double-clicking the title area, by the green
 * control's own menu, by a window manager, and by the user dragging it to an
 * edge — a flag we flipped ourselves would be wrong after any of those, and it
 * would be wrong in the one direction that matters, showing "come back" on a
 * window that is already back.
 *
 * `resize` is the one event every route has in common, and it is cheap: the
 * answer is a single boolean from a binding already open.
 */
export function useWindowMaximised(): boolean {
  const [maximised, setMaximised] = useState(false);

  useEffect(() => {
    let live = true;
    const read = () => {
      void getContainer()
        .desktop.isWindowMaximised()
        .then((value) => {
          if (live) setMaximised(value);
        });
    };
    read();
    addEventListener("resize", read);
    return () => {
      live = false;
      removeEventListener("resize", read);
    };
  }, []);

  return maximised;
}
