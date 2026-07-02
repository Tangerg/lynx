import { useMemo } from "react";
import {
  selectAgentSession,
  useCreateSession,
  useDeleteSession,
  useForkSession,
  useRenameSession,
  useToggleFavorite,
} from "@/plugins/builtin/agent/public/session";
import {
  openContextDockLauncher,
  openWorkspaceView,
} from "@/plugins/builtin/workspace/public/navigation";

export interface WorkIndexActions {
  createSession: () => void;
  startSessionInFolder: (cwd: string) => void;
  selectSession: (id: string) => void;
  renameSession: (id: string, title: string) => void;
  forkSession: (id: string) => void;
  deleteSession: (id: string) => void;
  toggleFavorite: (id: string, favorite: boolean) => void;
  openContextDock: () => void;
  openSettings: (title: string) => void;
}

export function useWorkIndexActions(): WorkIndexActions {
  const create = useCreateSession();
  const remove = useDeleteSession();
  const fork = useForkSession();
  const rename = useRenameSession();
  const toggleFavorite = useToggleFavorite();

  return useMemo(
    () => ({
      createSession: () => {
        void create();
      },
      startSessionInFolder: (cwd) => {
        void create({ cwd });
      },
      selectSession: selectAgentSession,
      renameSession: (id, title) => {
        void rename(id, title);
      },
      forkSession: (id) => {
        void fork(id);
      },
      deleteSession: (id) => {
        void remove(id);
      },
      toggleFavorite: (id, favorite) => {
        void toggleFavorite(id, favorite);
      },
      openContextDock: openContextDockLauncher,
      openSettings: (title) => {
        openWorkspaceView({ id: "settings", title, icon: "settings" });
      },
    }),
    [create, fork, remove, rename, toggleFavorite],
  );
}
