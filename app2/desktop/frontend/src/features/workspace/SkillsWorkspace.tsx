import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useCallback, useEffect, useRef, useState } from "react";

import type {
  ManagedSkill,
  RuntimeConnection,
  Skill,
  SkillProposal,
  WorkspaceRef,
} from "@lyra/runtime-contract";

import {
  approveSkillProposal,
  archiveSkill,
  listDiscoveredSkills,
  listManagedSkills,
  listSkillProposals,
  rejectSkillProposal,
  restoreSkill,
  runtimeQueryKeys,
} from "../../runtime/runtimeQueries";
import type { SkillView } from "./contextDockState";

interface SkillsWorkspaceProps {
  connection: RuntimeConnection;
  workspace: WorkspaceRef;
  enabled: boolean;
  view: SkillView;
  onViewChange(view: SkillView): void;
}

interface SkillAction {
  key: string;
  error?: string;
}

export function SkillsWorkspace(props: SkillsWorkspaceProps) {
  const queryClient = useQueryClient();
  const actionController = useRef<AbortController | undefined>(undefined);
  const [action, setAction] = useState<SkillAction>();
  const available = useQuery({
    queryKey: runtimeQueryKeys.discoveredSkills(
      props.connection,
      props.workspace.path,
    ),
    queryFn: ({ signal }) =>
      listDiscoveredSkills(props.connection, props.workspace, signal),
    enabled: props.enabled,
    retry: 2,
  });
  const proposals = useQuery({
    queryKey: runtimeQueryKeys.skillProposals(
      props.connection,
      props.workspace.path,
    ),
    queryFn: ({ signal }) =>
      listSkillProposals(props.connection, props.workspace, signal),
    enabled: props.enabled,
    retry: 2,
  });
  const library = useQuery({
    queryKey: runtimeQueryKeys.skillLibrary(props.connection),
    queryFn: ({ signal }) => listManagedSkills(props.connection, signal),
    enabled: props.enabled,
    retry: 2,
  });

  useEffect(() => {
    actionController.current?.abort();
    actionController.current = undefined;
    setAction(undefined);
    return () => actionController.current?.abort();
  }, [
    props.connection.generation,
    props.connection.instanceId,
    props.workspace.path,
  ]);

  const runAction = useCallback(
    async (
      key: string,
      operation: (signal: AbortSignal) => Promise<void>,
    ) => {
      actionController.current?.abort();
      const controller = new AbortController();
      actionController.current = controller;
      setAction({ key });
      try {
        await operation(controller.signal);
        await queryClient.invalidateQueries({
          queryKey: runtimeQueryKeys.skills(props.connection),
        });
        if (actionController.current === controller) setAction(undefined);
      } catch (error) {
        if (controller.signal.aborted) return;
        if (actionController.current === controller) {
          setAction({ key, error: messageOf(error) });
        }
      }
    },
    [props.connection, queryClient],
  );

  if (!props.enabled) {
    return (
      <SkillsState
        title="Skills unavailable"
        detail="This Runtime does not advertise the Lyra Skills capability."
      />
    );
  }

  return (
    <section className="skills-workspace" aria-label="Skills workspace">
      <nav className="skill-view-switch" aria-label="Skill views">
        <SkillViewButton
          label="Available"
          count={available.data?.data.length}
          selected={props.view === "available"}
          onSelect={() => props.onViewChange("available")}
        />
        <SkillViewButton
          label="Proposals"
          count={proposals.data?.data.length}
          selected={props.view === "proposals"}
          onSelect={() => props.onViewChange("proposals")}
        />
        <SkillViewButton
          label="Library"
          count={library.data?.data.length}
          selected={props.view === "library"}
          onSelect={() => props.onViewChange("library")}
        />
      </nav>
      {action?.error ? (
        <div className="skill-action-error" role="alert">
          <span>{action.error}</span>
          <button type="button" onClick={() => setAction(undefined)}>
            Dismiss
          </button>
        </div>
      ) : null}
      {props.view === "available" ? (
        <AvailableSkills
          values={available.data?.data}
          pending={available.isPending}
          error={available.error}
          onRetry={() => void available.refetch()}
        />
      ) : props.view === "proposals" ? (
        <SkillProposals
          values={proposals.data?.data}
          pending={proposals.isPending}
          error={proposals.error}
          actionKey={action?.key}
          onRetry={() => void proposals.refetch()}
          onApprove={(proposal) =>
            runAction(proposalActionKey(proposal, "approve"), (signal) =>
              approveSkillProposal(
                props.connection,
                { workspace: props.workspace, ...proposalRef(proposal) },
                signal,
              ),
            )
          }
          onReject={(proposal) =>
            runAction(proposalActionKey(proposal, "reject"), (signal) =>
              rejectSkillProposal(
                props.connection,
                { workspace: props.workspace, ...proposalRef(proposal) },
                signal,
              ),
            )
          }
        />
      ) : (
        <ManagedSkillLibrary
          values={library.data?.data}
          pending={library.isPending}
          error={library.error}
          actionKey={action?.key}
          onRetry={() => void library.refetch()}
          onArchive={(skill) =>
            runAction(`archive:${skill.name}`, (signal) =>
              archiveSkill(props.connection, skill.name, signal),
            )
          }
          onRestore={(skill) =>
            runAction(`restore:${skill.name}`, (signal) =>
              restoreSkill(props.connection, skill.name, signal),
            )
          }
        />
      )}
    </section>
  );
}

function SkillViewButton(props: {
  label: string;
  count?: number;
  selected: boolean;
  onSelect(): void;
}) {
  return (
    <button
      type="button"
      aria-current={props.selected ? "page" : undefined}
      onClick={props.onSelect}
    >
      <span>{props.label}</span>
      {props.count === undefined ? null : <small>{props.count}</small>}
    </button>
  );
}

function AvailableSkills(props: {
  values?: Skill[];
  pending: boolean;
  error: Error | null;
  onRetry(): void;
}) {
  if (props.pending) return <SkillsState title="Discovering Skills…" />;
  if (props.error) {
    return (
      <SkillsState
        title="Skills could not be discovered"
        detail={messageOf(props.error)}
        action="Try again"
        onAction={props.onRetry}
      />
    );
  }
  if (!props.values || props.values.length === 0) {
    return (
      <SkillsState
        title="No Skills available"
        detail="Add a valid SKILL.md under .lyra/skills, or approve a pending proposal."
      />
    );
  }
  return (
    <div className="skill-card-list">
      {props.values.map((skill) => (
        <article className="skill-card" key={`${skill.scope}:${skill.name}`}>
          <header>
            <h4>{skill.name}</h4>
            <SkillTag>{skill.scope}</SkillTag>
          </header>
          <p>{skill.description || "No description provided."}</p>
        </article>
      ))}
    </div>
  );
}

function SkillProposals(props: {
  values?: SkillProposal[];
  pending: boolean;
  error: Error | null;
  actionKey?: string;
  onRetry(): void;
  onApprove(proposal: SkillProposal): Promise<void>;
  onReject(proposal: SkillProposal): Promise<void>;
}) {
  if (props.pending) return <SkillsState title="Loading proposals…" />;
  if (props.error) {
    return (
      <SkillsState
        title="Proposals could not be loaded"
        detail={messageOf(props.error)}
        action="Try again"
        onAction={props.onRetry}
      />
    );
  }
  if (!props.values || props.values.length === 0) {
    return (
      <SkillsState
        title="No pending proposals"
        detail="Agent-authored Skills remain inactive until a proposal appears here and you approve its exact revision."
      />
    );
  }
  return (
    <div className="skill-card-list">
      {props.values.map((proposal) => {
        const approveKey = proposalActionKey(proposal, "approve");
        const rejectKey = proposalActionKey(proposal, "reject");
        const busy = props.actionKey !== undefined;
        return (
          <article
            className="skill-card skill-proposal-card"
            key={`${proposal.scope}:${proposal.name}:${proposal.revision}`}
          >
            <header>
              <h4>{proposal.name}</h4>
              <span className="skill-tags">
                <SkillTag>{proposal.scope}</SkillTag>
                {proposal.revises ? <SkillTag>revision</SkillTag> : null}
              </span>
            </header>
            <p>{proposal.description}</p>
            <dl className="skill-proposal-facts">
              <div>
                <dt>Origin</dt>
                <dd>{proposal.origin || "unknown"}</dd>
              </div>
              <div>
                <dt>Session</dt>
                <dd title={proposal.sourceSession}>{proposal.sourceSession || "—"}</dd>
              </div>
              <div>
                <dt>Revision</dt>
                <dd title={proposal.revision}>
                  <code>{proposal.revision.slice(0, 12)}</code>
                </dd>
              </div>
            </dl>
            <details open>
              <summary>Complete instructions</summary>
              <pre>{proposal.instructions}</pre>
            </details>
            <footer>
              <button
                className="secondary-action"
                type="button"
                disabled={busy}
                onClick={() => void props.onReject(proposal)}
              >
                {props.actionKey === rejectKey ? "Rejecting…" : "Reject"}
              </button>
              <button
                className="primary-action"
                type="button"
                disabled={busy}
                onClick={() => void props.onApprove(proposal)}
              >
                {props.actionKey === approveKey ? "Approving…" : "Approve exact revision"}
              </button>
            </footer>
          </article>
        );
      })}
    </div>
  );
}

function ManagedSkillLibrary(props: {
  values?: ManagedSkill[];
  pending: boolean;
  error: Error | null;
  actionKey?: string;
  onRetry(): void;
  onArchive(skill: ManagedSkill): Promise<void>;
  onRestore(skill: ManagedSkill): Promise<void>;
}) {
  if (props.pending) return <SkillsState title="Loading user library…" />;
  if (props.error) {
    return (
      <SkillsState
        title="Skill library could not be loaded"
        detail={messageOf(props.error)}
        action="Try again"
        onAction={props.onRetry}
      />
    );
  }
  if (!props.values || props.values.length === 0) {
    return (
      <SkillsState
        title="User library is empty"
        detail="Approved user-scoped Skills and externally authored ~/.lyra/skills bundles appear here."
      />
    );
  }
  const active = props.values.filter((skill) => skill.lifecycle === "active");
  const archived = props.values.filter((skill) => skill.lifecycle === "archived");
  return (
    <div className="managed-skill-library">
      <ManagedSkillSection
        title="Active"
        empty="No active user Skills."
        values={active}
        actionKey={props.actionKey}
        action="Archive"
        onAction={props.onArchive}
      />
      <ManagedSkillSection
        title="Archived"
        empty="No archived Skills."
        values={archived}
        actionKey={props.actionKey}
        action="Restore"
        onAction={props.onRestore}
      />
    </div>
  );
}

function ManagedSkillSection(props: {
  title: string;
  empty: string;
  values: ManagedSkill[];
  actionKey?: string;
  action: "Archive" | "Restore";
  onAction(skill: ManagedSkill): Promise<void>;
}) {
  return (
    <section className="managed-skill-section">
      <header>
        <h4>{props.title}</h4>
        <small>{props.values.length}</small>
      </header>
      {props.values.length === 0 ? (
        <p className="managed-skill-empty">{props.empty}</p>
      ) : (
        <div className="skill-card-list">
          {props.values.map((skill) => {
            const key = `${props.action.toLowerCase()}:${skill.name}`;
            return (
              <article className="skill-card" key={skill.name}>
                <header>
                  <h5>{skill.name}</h5>
                  <button
                    className="secondary-action"
                    type="button"
                    disabled={props.actionKey !== undefined}
                    onClick={() => void props.onAction(skill)}
                  >
                    {props.actionKey === key ? `${props.action}…` : props.action}
                  </button>
                </header>
                <p>{skill.description || "No description provided."}</p>
              </article>
            );
          })}
        </div>
      )}
    </section>
  );
}

function SkillsState(props: {
  title: string;
  detail?: string;
  action?: string;
  onAction?(): void;
}) {
  return (
    <div className="skills-state">
      <h4>{props.title}</h4>
      {props.detail ? <p>{props.detail}</p> : null}
      {props.action && props.onAction ? (
        <button className="secondary-action" type="button" onClick={props.onAction}>
          {props.action}
        </button>
      ) : null}
    </div>
  );
}

function SkillTag(props: { children: string }) {
  return <small className="skill-tag">{props.children}</small>;
}

function proposalRef(proposal: SkillProposal) {
  return {
    name: proposal.name,
    revision: proposal.revision,
    scope: proposal.scope,
  };
}

function proposalActionKey(
  proposal: SkillProposal,
  action: "approve" | "reject",
) {
  return `${action}:${proposal.scope}:${proposal.name}:${proposal.revision}`;
}

function messageOf(error: unknown) {
  return error instanceof Error ? error.message : "Unexpected Runtime failure";
}
