type PluginActivator = (pluginName: string) => Promise<void>;

let activateConfiguredPlugin: PluginActivator | null = null;

/**
 * Connect lazy selectors to the plugin loader without creating a module cycle.
 * The SDK configures this once while its plugin runtime is initialized.
 */
export function configurePluginActivation(activate: PluginActivator): void {
  activateConfiguredPlugin = activate;
}

/** Activate the plugin that owns a declared placeholder. */
export async function activatePlugin(pluginName: string): Promise<void> {
  if (!activateConfiguredPlugin) {
    console.error(`[plugin] activation is not configured; cannot activate ${pluginName}`);
    return;
  }
  await activateConfiguredPlugin(pluginName);
}
