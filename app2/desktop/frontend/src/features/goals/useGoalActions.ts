import { useMutation, useQueryClient } from "@tanstack/react-query";

import type {
  Goal,
  GoalBudget,
  RuntimeConnection,
  SessionSnapshot,
} from "@lyra/runtime-contract";

import {
  clearGoal,
  pauseGoal,
  resumeGoal,
  runtimeQueryKeys,
  startGoal,
  updateGoal,
} from "../../runtime/runtimeQueries";

export function useGoalActions(
  connection: RuntimeConnection,
  selectedSessionId: string | undefined,
) {
  const queryClient = useQueryClient();

  const refresh = (sessionId: string) => {
    void queryClient.invalidateQueries({
      queryKey: runtimeQueryKeys.snapshot(connection, sessionId),
    });
    void queryClient.invalidateQueries({
      queryKey: runtimeQueryKeys.sessions(connection),
    });
  };
  const commit = (goal: Goal) => {
    queryClient.setQueryData<SessionSnapshot>(
      runtimeQueryKeys.snapshot(connection, goal.sessionId),
      (snapshot) => (snapshot ? { ...snapshot, goal } : snapshot),
    );
    refresh(goal.sessionId);
  };

  const start = useMutation({
    mutationFn: (request: {
      sessionId: string;
      objective: string;
      budget?: GoalBudget;
    }) =>
      startGoal(
        connection,
        request.sessionId,
        request.objective,
        request.budget,
      ),
    onSuccess: commit,
  });
  const update = useMutation({
    mutationFn: (request: { sessionId: string; objective: string }) =>
      updateGoal(connection, request.sessionId, request.objective),
    onSuccess: commit,
  });
  const pause = useMutation({
    mutationFn: (sessionId: string) => pauseGoal(connection, sessionId),
    onSuccess: commit,
  });
  const resume = useMutation({
    mutationFn: (sessionId: string) => resumeGoal(connection, sessionId),
    onSuccess: commit,
  });
  const clear = useMutation({
    mutationFn: (sessionId: string) => clearGoal(connection, sessionId),
    onSuccess: (_result, sessionId) => {
      queryClient.setQueryData<SessionSnapshot>(
        runtimeQueryKeys.snapshot(connection, sessionId),
        (snapshot) => (snapshot ? { ...snapshot, goal: undefined } : snapshot),
      );
      refresh(sessionId);
    },
  });

  const states = [
    {
      sessionId: start.variables?.sessionId,
      pending: start.isPending,
      error: start.error,
    },
    {
      sessionId: update.variables?.sessionId,
      pending: update.isPending,
      error: update.error,
    },
    {
      sessionId: pause.variables,
      pending: pause.isPending,
      error: pause.error,
    },
    {
      sessionId: resume.variables,
      pending: resume.isPending,
      error: resume.error,
    },
    {
      sessionId: clear.variables,
      pending: clear.isPending,
      error: clear.error,
    },
  ].filter((state) => state.sessionId === selectedSessionId);
  return {
    start: start.mutateAsync,
    update: update.mutateAsync,
    pause: pause.mutateAsync,
    resume: resume.mutateAsync,
    clear: clear.mutateAsync,
    pending: states.some((state) => state.pending),
    error: states.find((state) => state.error)?.error,
  };
}

export type GoalActions = ReturnType<typeof useGoalActions>;
