import { DesktopBridgeError } from "../../runtime/desktopBridge";
import { RuntimeDiscoveryError } from "../../runtime/runtimeQueries";
import type { Translate } from "./Localization";

export function presentRuntimeError(
  error: unknown,
  fallback: string,
  t: Translate,
): string {
  if (error instanceof DesktopBridgeError) {
    switch (error.code) {
      case "invalidRuntimeBootstrap":
        return t("runtimeError.invalidRuntimeBootstrap");
      case "unsafeRuntimeEndpoint":
        return t("runtimeError.unsafeRuntimeEndpoint");
      case "hostUnavailable":
        return t("runtimeError.hostUnavailable");
      case "invalidBootstrapEnvelope":
        return t("runtimeError.invalidBootstrapEnvelope");
      case "invalidRemoteRuntimeState":
        return t("runtimeError.invalidRemoteRuntimeState");
      case "inconsistentRemoteRuntimeState":
        return t("runtimeError.inconsistentRemoteRuntimeState");
      case "invalidDirectorySelection":
        return t("runtimeError.invalidDirectorySelection");
      case "invalidImageSaveResult":
        return t("runtimeError.invalidImageSaveResult");
      case "invalidSessionArtifactSelection":
        return t("runtimeError.invalidSessionArtifactSelection");
      case "invalidSessionExportResult":
        return t("runtimeError.invalidSessionExportResult");
    }
  }
  if (error instanceof RuntimeDiscoveryError) {
    switch (error.code) {
      case "identityChanged":
        return t("runtimeError.identityChanged");
    }
  }
  return error instanceof Error ? error.message : fallback;
}
