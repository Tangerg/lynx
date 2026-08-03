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

/**
 * Where the platform put the window's own controls, in CSS pixels from the window's
 * top-left.
 *
 * `null` means there was nothing to ask — a browser tab, a visual fixture, a
 * platform whose controls sit outside the content — and the stylesheet's own header
 * height and gutter stand. A measured `controlsInlineEnd` of 0 is a different answer
 * and a real one: the window is fullscreen and the marks are gone with the menu bar.
 */
export interface WindowChrome {
  /** Distance down from the window's top to the marks' centre line — what a control
   *  beside them centres on. */
  controlsCentreY: number;
  /** Where the cluster ends, and so where the header's first control may begin. */
  controlsInlineEnd: number;
}

interface DesktopHostBinding {
  Bootstrap(): Promise<unknown>;
  WindowChrome(): Promise<unknown>;
}

export interface DesktopHostClient {
  /** Returns null in a plain browser where the Wails host is intentionally absent. */
  bootstrap(): Promise<DesktopBootstrap | null>;
  /** `null` where there is no window to measure. Re-read per layout: the titlebar is
   *  rebuilt entering and leaving fullscreen, and the marks go away with it. */
  windowChrome(): Promise<WindowChrome | null>;
}

const WindowChromeSchema = z.object({
  controlsCentreY: z.number().nonnegative(),
  controlsInlineEnd: z.number().nonnegative(),
  measured: z.boolean(),
});

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
    async windowChrome() {
      const host = binding ?? wailsDesktopHostBinding();
      if (!host) return null;
      // Geometry the layout can fall back on: a throw here, or a shape the host
      // no longer speaks, must leave the stylesheet's declared values standing
      // rather than collapse the header to zero.
      try {
        const parsed = WindowChromeSchema.safeParse(await host.WindowChrome());
        if (!parsed.success || !parsed.data.measured) return null;
        return {
          controlsCentreY: parsed.data.controlsCentreY,
          controlsInlineEnd: parsed.data.controlsInlineEnd,
        };
      } catch {
        return null;
      }
    },
  };
}
