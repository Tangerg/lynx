import { composerState } from "../application/ports/state";

export function useRecordComposerHistory(): (text: string) => void {
  return composerState().useRecordHistory();
}

export function recallPreviousComposerHistory(): boolean {
  return composerState().recallPreviousHistory();
}

export function recallNextComposerHistory(): boolean {
  return composerState().recallNextHistory();
}
