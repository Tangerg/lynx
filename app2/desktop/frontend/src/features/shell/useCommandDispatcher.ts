import { useEffect, useRef } from "react";

import {
  isEditableCommandTarget,
  matchesCommandShortcut,
  type CommandDescriptor,
} from "./commandCatalog";

export interface CommandBinding {
  descriptor: CommandDescriptor;
  enabled: boolean;
  run(): void | Promise<void>;
}

export function useCommandDispatcher(options: {
  active: boolean;
  commands: readonly CommandBinding[];
  onError(command: CommandDescriptor, error: unknown): void;
}) {
  const commands = useRef(options.commands);
  const onError = useRef(options.onError);
  commands.current = options.commands;
  onError.current = options.onError;

  useEffect(() => {
    if (!options.active) return;
    const dispatch = (event: KeyboardEvent) => {
      if (document.querySelector('[role="dialog"]') !== null) return;
      for (const binding of commands.current) {
        const { descriptor } = binding;
        if (
          !binding.enabled ||
          (!descriptor.shortcut.allowInEditable &&
            isEditableCommandTarget(event.target)) ||
          !matchesCommandShortcut(descriptor.shortcut, event)
        ) {
          continue;
        }
        event.preventDefault();
        void Promise.resolve()
          .then(() => binding.run())
          .catch((error) => onError.current(descriptor, error));
        return;
      }
    };
    window.addEventListener("keydown", dispatch);
    return () => window.removeEventListener("keydown", dispatch);
  }, [options.active]);
}
