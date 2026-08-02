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
}

export interface DesktopHostClient {
  /** Returns null in a plain browser where the Wails host is intentionally absent. */
  bootstrap(): Promise<DesktopBootstrap | null>;
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
  };
}
