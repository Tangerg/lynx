import { z } from "zod";
import { errorMessage, RpcTransportError } from "./errors";

export interface LocalRuntimeConnection {
  endpoint: string;
  localToken?: string;
}

export interface SideloadedPlugin {
  id: string;
  source: string;
}

export interface SideloadIssue {
  id: string;
  detail: string;
}

export interface DesktopBootstrap {
  localRuntime: LocalRuntimeConnection;
  sideloadedPlugins: SideloadedPlugin[];
  sideloadIssues: SideloadIssue[];
}

interface DesktopHostBinding {
  Bootstrap(): Promise<unknown>;
  MinimiseWindow(): Promise<void>;
  ToggleMaximiseWindow(): Promise<void>;
  CloseWindow(): Promise<void>;
}

export interface DesktopHostClient {
  /** Returns null in a plain browser where the Wails host is intentionally absent. */
  bootstrap(): Promise<DesktopBootstrap | null>;
  /**
   * The three window commands, because the platform draws no controls for them.
   *
   * Void and fire-and-forget: each one either moves the window or the window is
   * not there (a browser tab, a visual fixture), and neither outcome is
   * something a caller can act on. Failing here must never surface as an error
   * in the UI — a dead minimise button is not worth a dialog.
   */
  minimiseWindow(): void;
  toggleMaximiseWindow(): void;
  closeWindow(): void;
}

const DesktopBootstrapSchema = z.object({
  localRuntime: z.object({
    endpoint: z.url(),
    localToken: z.string().min(1).optional(),
  }),
  sideloadedPlugins: z.array(z.object({ id: z.string().min(1), source: z.string().min(1) })),
  sideloadIssues: z.array(z.object({ id: z.string().min(1), detail: z.string().min(1) })),
});

function wailsDesktopHostBinding(): DesktopHostBinding | undefined {
  const root = globalThis as typeof globalThis & {
    go?: { main?: { DesktopHost?: DesktopHostBinding } };
  };
  return root.go?.main?.DesktopHost;
}

export function createDesktopHostClient(binding?: DesktopHostBinding): DesktopHostClient {
  let pending: Promise<DesktopBootstrap | null> | undefined;
  return {
    bootstrap() {
      pending ??= (async () => {
        const host = binding ?? wailsDesktopHostBinding();
        if (!host) return null;
        let value: unknown;
        try {
          value = await host.Bootstrap();
        } catch (error) {
          throw new RpcTransportError(`desktop host bootstrap failed: ${errorMessage(error)}`);
        }
        const parsed = DesktopBootstrapSchema.safeParse(value);
        if (!parsed.success) {
          throw new RpcTransportError(
            `desktop host bootstrap returned an invalid shape: ${parsed.error.message}`,
          );
        }
        return parsed.data;
      })();
      return pending;
    },
    minimiseWindow: () => command((host) => host.MinimiseWindow()),
    toggleMaximiseWindow: () => command((host) => host.ToggleMaximiseWindow()),
    closeWindow: () => command((host) => host.CloseWindow()),
  };

  function command(run: (host: DesktopHostBinding) => Promise<void>): void {
    const host = binding ?? wailsDesktopHostBinding();
    if (!host) return;
    void Promise.resolve(run(host)).catch((error: unknown) => {
      console.error("[desktop] window command failed:", error);
    });
  }
}
