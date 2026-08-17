export interface MountedRenderer {
  unmount(): void;
}

export interface DesktopRendererDependencies {
  initializeDesktopHost(): Promise<void>;
  prepareWindowChrome(): Promise<void>;
  watchWindowChrome(): () => void;
  mount(): MountedRenderer;
  closeRuntime(): Promise<void>;
  reportFailure(
    scope: "host bootstrap" | "React root teardown" | "window chrome teardown",
    error: unknown,
  ): void;
}

/** Owns one renderer generation from bootstrap admission through final close. */
export class DesktopRenderer {
  readonly #dependencies: DesktopRendererDependencies;
  #active = true;
  #startup: Promise<void> | undefined;
  #mounted: MountedRenderer | undefined;
  #stopWatchingWindowChrome: (() => void) | undefined;
  #closing: Promise<void> | undefined;

  constructor(dependencies: DesktopRendererDependencies) {
    this.#dependencies = dependencies;
  }

  start(): Promise<void> {
    if (this.#startup) return this.#startup;
    if (!this.#active) return Promise.resolve();
    const startup = this.#startOwned().catch(async (error: unknown) => {
      await this.dispose();
      throw error;
    });
    this.#startup = startup;
    return startup;
  }

  async #startOwned(): Promise<void> {
    try {
      await this.#dependencies.initializeDesktopHost();
    } catch (error) {
      if (this.#active) this.#dependencies.reportFailure("host bootstrap", error);
    }
    if (!this.#active) return;

    await this.#dependencies.prepareWindowChrome();
    if (!this.#active) return;

    const stopWatching = this.#dependencies.watchWindowChrome();
    if (!this.#active) {
      stopWatching();
      return;
    }
    this.#stopWatchingWindowChrome = stopWatching;

    const mounted = this.#dependencies.mount();
    if (!this.#active) {
      mounted.unmount();
      return;
    }
    this.#mounted = mounted;
  }

  dispose(): Promise<void> {
    if (this.#closing) return this.#closing;
    this.#active = false;

    const mounted = this.#mounted;
    this.#mounted = undefined;
    try {
      mounted?.unmount();
    } catch (error) {
      this.#dependencies.reportFailure("React root teardown", error);
    }

    const stopWatching = this.#stopWatchingWindowChrome;
    this.#stopWatchingWindowChrome = undefined;
    try {
      stopWatching?.();
    } catch (error) {
      this.#dependencies.reportFailure("window chrome teardown", error);
    }

    try {
      this.#closing = this.#dependencies.closeRuntime();
    } catch (error) {
      this.#closing = Promise.reject(error);
    }
    return this.#closing;
  }
}
