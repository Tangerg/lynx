import { useMemo } from "react";
import {
  selectAgentSession,
  useCreateSession,
  useDeleteSession,
  useForkSession,
  useRenameSession,
  useToggleFavorite,
} from "@/plugins/builtin/agent/public/session";
import { focusComposer } from "@/plugins/builtin/chat/composer/public/focus";
import { showWorkspaceDock } from "@/plugins/builtin/workspace/public/navigation";
import { openSettingsView } from "@/plugins/builtin/workspace/public/deeplinks";

export interface WorkIndexActions {
  createSession: () => void;
  startSessionInFolder: (cwd: string) => void;
  selectSession: (id: string) => void;
  renameSession: (id: string, expectedRevision: number, title: string) => void;
  forkSession: (id: string) => void;
  deleteSession: (id: string) => void;
  toggleFavorite: (id: string, expectedRevision: number, favorite: boolean) => void;
  openContextDock: () => void;
  openSettings: () => void;
}

export function useWorkIndexActions(): WorkIndexActions {
  const create = useCreateSession();
  const remove = useDeleteSession();
  const fork = useForkSession();
  const rename = useRenameSession();
  const toggleFavorite = useToggleFavorite();

  return useMemo(
    () => ({
      // A fresh session may already be on screen, in which case create() hands
      // that one back and nothing moves — so the caret is the acknowledgement.
      createSession: () => {
        void create().then(() => focusComposer());
      },
      startSessionInFolder: (cwd) => {
        void create({ cwd }).then(() => focusComposer());
      },
      selectSession: selectAgentSession,
      renameSession: (id, expectedRevision, title) => {
        void rename(id, expectedRevision, title);
      },
      forkSession: (id) => {
        void fork(id);
      },
      deleteSession: (id) => {
        void remove(id);
      },
      toggleFavorite: (id, expectedRevision, favorite) => {
        void toggleFavorite(id, expectedRevision, favorite);
      },
      openContextDock: showWorkspaceDock,
      openSettings: () => {
        openSettingsView();
      },
    }),
    [create, fork, remove, rename, toggleFavorite],
  );
}
