import { getContainer } from "@/main/container";
import { configureWorkingDirectoryPicker } from "../application/ports/workingDirectoryPicker";

export function installDesktopWorkingDirectoryPicker(): () => void {
  return configureWorkingDirectoryPicker({
    choose: () => getContainer().desktop.chooseWorkingDirectory(),
  });
}
