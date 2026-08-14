import { createSingletonPort } from "@/lib/ports/singletonPort";

export interface WorkingDirectoryPicker {
  /** Returns null when the native chooser is cancelled or unavailable. */
  choose(): Promise<string | null>;
}

const port = createSingletonPort<WorkingDirectoryPicker>(
  "Working directory picker is not configured",
);

export const configureWorkingDirectoryPicker = port.configure;
export const workingDirectoryPicker = port.get;
