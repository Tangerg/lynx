import {
  useInfiniteQuery,
  useMutation,
  useQueryClient,
  type InfiniteData,
} from "@tanstack/react-query";
import { useMemo } from "react";

import type {
  CreateSessionRequest,
  Page,
  RuntimeConnection,
  Session,
  SessionArtifact,
  UpdateSessionRequest,
} from "@lyra/runtime-contract";

import {
  createSession,
  deleteSession,
  forkSession,
  importSession,
  listSessions,
  runtimeQueryKeys,
  updateSession,
} from "../../runtime/runtimeQueries";

type SessionPages = InfiniteData<Page<Session>, string | undefined>;
type SessionPatch = Omit<
  UpdateSessionRequest,
  "sessionId" | "expectedRevision"
>;

export function useSessionCatalog(connection: RuntimeConnection) {
  const queryClient = useQueryClient();
  const queryKey = runtimeQueryKeys.sessions(connection);
  const query = useInfiniteQuery({
    queryKey,
    queryFn: ({ pageParam, signal }) =>
      listSessions(connection, pageParam, signal),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (page) => page.nextCursor,
    retry: 2,
  });
  const sessions = useMemo(
    () => uniqueSessions(query.data?.pages.flatMap((page) => page.data) ?? []),
    [query.data],
  );

  const create = useMutation({
    mutationFn: (request: CreateSessionRequest = {}) =>
      createSession(connection, request),
    onSuccess: (committed) => {
      queryClient.setQueryData<SessionPages>(queryKey, (current) =>
        upsertSession(current, committed, true),
      );
      void queryClient.invalidateQueries({ queryKey });
    },
  });
  const update = useMutation({
    mutationFn: ({ source, patch }: { source: Session; patch: SessionPatch }) =>
      updateSession(connection, {
        sessionId: source.id,
        expectedRevision: source.revision,
        ...patch,
      }),
    onSuccess: (committed) => {
      queryClient.setQueryData<SessionPages>(queryKey, (current) =>
        upsertSession(current, committed, false),
      );
      void queryClient.invalidateQueries({ queryKey });
    },
  });
  const remove = useMutation({
    mutationFn: async (source: Session) => {
      await deleteSession(connection, source.id);
      return source.id;
    },
    onSuccess: (sessionId) => {
      queryClient.setQueryData<SessionPages>(queryKey, (current) =>
        removeSession(current, sessionId),
      );
      queryClient.removeQueries({
        queryKey: runtimeQueryKeys.snapshot(connection, sessionId),
        exact: true,
      });
      void queryClient.invalidateQueries({ queryKey });
    },
  });
  const fork = useMutation({
    mutationFn: ({
      source,
      fromRunId,
    }: {
      source: Session;
      fromRunId?: string;
    }) =>
      forkSession(connection, {
        sessionId: source.id,
        ...(fromRunId === undefined ? {} : { fromRunId }),
      }),
    onSuccess: (committed) => {
      queryClient.setQueryData<SessionPages>(queryKey, (current) =>
        upsertSession(current, committed, true),
      );
      void queryClient.invalidateQueries({ queryKey });
    },
  });
  const importArtifact = useMutation({
    mutationFn: (artifact: SessionArtifact) =>
      importSession(connection, artifact),
    onSuccess: ({ session: committed }) => {
      queryClient.setQueryData<SessionPages>(queryKey, (current) =>
        upsertSession(current, committed, true),
      );
      void queryClient.invalidateQueries({ queryKey });
    },
  });

  return {
    query,
    sessions,
    create: create.mutateAsync,
    update: update.mutateAsync,
    remove: remove.mutateAsync,
    fork: fork.mutateAsync,
    importArtifact: importArtifact.mutateAsync,
    createPending: create.isPending,
    updatePending: update.isPending,
    removePending: remove.isPending,
    forkPending: fork.isPending,
    importPending: importArtifact.isPending,
    createError: create.error,
  };
}

function uniqueSessions(sessions: Session[]): Session[] {
  const seen = new Set<string>();
  return sessions.filter((session) => {
    if (seen.has(session.id)) return false;
    seen.add(session.id);
    return true;
  });
}

function upsertSession(
  current: SessionPages | undefined,
  committed: Session,
  prepend: boolean,
): SessionPages {
  if (!current) {
    return { pages: [{ data: [committed] }], pageParams: [undefined] };
  }
  let found = false;
  const pages = current.pages.map((page) => ({
    ...page,
    data: page.data.map((session) => {
      if (session.id !== committed.id) return session;
      found = true;
      return committed;
    }),
  }));
  if (!found || prepend) {
    pages[0] = {
      ...pages[0],
      data: [
        committed,
        ...pages[0].data.filter((session) => session.id !== committed.id),
      ],
    };
  }
  return { ...current, pages };
}

function removeSession(
  current: SessionPages | undefined,
  sessionId: string,
): SessionPages | undefined {
  if (!current) return current;
  return {
    ...current,
    pages: current.pages.map((page) => ({
      ...page,
      data: page.data.filter((session) => session.id !== sessionId),
    })),
  };
}
