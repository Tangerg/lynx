// Window bridge — gives sideloaded plugin bundles access to the host's
// React, motion, and SDK singletons without requiring them to bundle their
// own copies.
//
// Static imports here, not dynamic — Vite was splitting React into its own
// chunk under dynamic imports, and even though ESM should de-dupe in spec,
// the dev-mode load order can shake out wrong (you end up with two React
// instances visible during a render, which gives you wonderful errors like
// "dispatcher.useRef is null" and "maximum update depth"). Static keeps
// everything in the main chunk.

import * as Motion from "motion/react";
import * as React from "react";
import * as ReactJSXRuntime from "react/jsx-runtime";
import * as SDK from "@/plugins/sdk";
import { disposeOnHmr } from "@/lib/hmr";
import { HOST_API_VERSION } from "../sdk/apiVersion";
import { safeCall } from "../sdk/errors";
import { BEFORE_UNLOAD_HANDLER } from "../sdk/kernelPoints";
import { lookupExtensionOwnedEntries } from "../sdk/selectors/extensions";

declare global {
  interface Window {
    __LYRA__?: LyraHostBridge;
  }
}

export interface LyraHostBridge {
  apiVersion: string;
  React: typeof React;
  ReactJSXRuntime: typeof ReactJSXRuntime;
  Motion: typeof Motion;
  SDK: typeof SDK;
}

let bridgeInstalled = false;
let beforeUnloadHandler: (() => void) | null = null;

export function installHostBridge(): void {
  if (typeof window === "undefined") return;
  window.__LYRA__ = {
    apiVersion: HOST_API_VERSION,
    React,
    ReactJSXRuntime,
    Motion,
    SDK,
  };
  if (bridgeInstalled) return;
  bridgeInstalled = true;
  // Single beforeunload listener — fans out to every plugin-registered
  // BeforeUnloadHandler. Synchronous on purpose: browsers don't await
  // promises during unload. Guarded by `bridgeInstalled` so React strict-
  // mode's double-mounted effect doesn't stack duplicate listeners.
  beforeUnloadHandler = () => {
    for (const o of lookupExtensionOwnedEntries(BEFORE_UNLOAD_HANDLER)) {
      safeCall(() => o.item(), `[plugin] ${o.pluginName} onBeforeUnload threw:`);
    }
  };
  window.addEventListener("beforeunload", beforeUnloadHandler);
}

disposeOnHmr(() => {
  if (beforeUnloadHandler) {
    window.removeEventListener("beforeunload", beforeUnloadHandler);
    beforeUnloadHandler = null;
  }
  bridgeInstalled = false;
});
