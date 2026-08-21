import { z } from "zod";
import { errorMessage, RpcTransportError } from "./errors";

export interface LocalRuntimeConnection {
  endpoint: string;
  localToken?: string;
}

export interface DesktopBootstrap {
  localRuntime: LocalRuntimeConnection;
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

/**
 * The four Go methods this app can reach, by the name the runtime knows them by.
 *
 * `package.Type.Method`, which for a `main` package is what Go's own reflection reports —
 * so these are the full names, not a shortened form. v3 can generate typed wrappers for
 * them instead; this calls by name deliberately, because a wrapper returns `any` and
 * validates nothing, and everything crossing this boundary is Zod-checked below.
 */
const HOST_METHOD = {
  bootstrap: "main.DesktopHost.Bootstrap",
  chooseWorkingDirectory: "main.DesktopHost.ChooseWorkingDirectory",
  saveImage: "main.DesktopHost.SaveImage",
  windowChrome: "main.DesktopHost.WindowChrome",
} as const;

/** How a call reaches Go. The app installs the Wails runtime's; tests pass their own. */
export interface DesktopHostBinding {
  call(method: string, ...args: unknown[]): Promise<unknown>;
}

export interface DesktopHostClient {
  /** Returns null in a plain browser where the Wails host is intentionally absent. */
  bootstrap(): Promise<DesktopBootstrap | null>;
  /** Opens the native directory chooser. `null` means the user cancelled or the
   *  packaged host is absent (for example, a browser visual fixture). */
  chooseWorkingDirectory(): Promise<string | null>;
  /** Opens the native save dialog for a rendered inline image. `false` means the
   *  user cancelled or the packaged host is absent; failures reject. */
  saveImage(source: string): Promise<boolean>;
  /** `null` where there is no window to measure. Re-read per layout: the titlebar is
   *  rebuilt entering and leaving fullscreen, and the marks go away with it. */
  windowChrome(): Promise<WindowChrome | null>;
}

const WindowChromeSchema = z.object({
  controlsCentreY: z.number().nonnegative(),
  controlsInlineEnd: z.number().nonnegative(),
  measured: z.boolean(),
});

const WorkingDirectorySchema = z.string();
const SaveImageSchema = z.boolean();

const DesktopBootstrapSchema = z.object({
  localRuntime: z.object({
    endpoint: z.url(),
    localToken: z.string().min(1).optional(),
  }),
});

/**
 * The Wails runtime, or nothing.
 *
 * `window._wails` is what the injected runtime installs, so its presence is the honest
 * test for "is there a host to ask". The import is dynamic because of what is on the
 * other side of that question: `@wailsio/runtime` has side effects on import — it
 * installs listeners and starts talking to a host — and this module is loaded in a plain
 * browser too, by the visual fixtures. Importing it eagerly would run all of that in a
 * page with no Wails behind it.
 */
async function wailsDesktopHostBinding(): Promise<DesktopHostBinding | undefined> {
  if (!("_wails" in globalThis)) return undefined;
  const { Call } = await import("@wailsio/runtime");
  return { call: (method, ...args) => Call.ByName(method, ...args) };
}

export function createDesktopHostClient(binding?: DesktopHostBinding): DesktopHostClient {
  let pending: Promise<DesktopBootstrap | null> | undefined;
  return {
    bootstrap() {
      pending ??= (async () => {
        const host = binding ?? (await wailsDesktopHostBinding());
        if (!host) return null;
        let value: unknown;
        try {
          value = await host.call(HOST_METHOD.bootstrap);
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
    async chooseWorkingDirectory() {
      const host = binding ?? (await wailsDesktopHostBinding());
      if (!host) return null;
      let value: unknown;
      try {
        value = await host.call(HOST_METHOD.chooseWorkingDirectory);
      } catch (error) {
        throw new RpcTransportError(
          `desktop host directory selection failed: ${errorMessage(error)}`,
        );
      }
      const parsed = WorkingDirectorySchema.safeParse(value);
      if (!parsed.success) {
        throw new RpcTransportError(
          `desktop host directory selection returned an invalid shape: ${parsed.error.message}`,
        );
      }
      return parsed.data.length > 0 ? parsed.data : null;
    },
    async saveImage(source) {
      const host = binding ?? (await wailsDesktopHostBinding());
      if (!host) return false;
      let value: unknown;
      try {
        value = await host.call(HOST_METHOD.saveImage, source);
      } catch (error) {
        throw new RpcTransportError(`desktop host image save failed: ${errorMessage(error)}`);
      }
      const parsed = SaveImageSchema.safeParse(value);
      if (!parsed.success) {
        throw new RpcTransportError(
          `desktop host image save returned an invalid shape: ${parsed.error.message}`,
        );
      }
      return parsed.data;
    },
    async windowChrome() {
      const host = binding ?? (await wailsDesktopHostBinding());
      if (!host) return null;
      // Geometry the layout can fall back on: a throw here, or a shape the host
      // no longer speaks, must leave the stylesheet's declared values standing
      // rather than collapse the header to zero.
      try {
        const parsed = WindowChromeSchema.safeParse(await host.call(HOST_METHOD.windowChrome));
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
