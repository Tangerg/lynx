// Add / edit form for one MCP server. Transport segmented control switches the
// dynamic field set: stdio = command + args (one per line) + env (KEY=value per
// line) + dir; http = url + authorization (password, "leave blank to keep").
// Save → configure; Test → live probe with an inline ok/err chip; Delete on an
// existing server. Mirrors the providers pane's save/test/probe-token flow.

import type { MCPServerConfigInfo, MCPTransport } from "@/lib/data/queries";
import type { ConfigureMCPServerRequest } from "@/rpc";
import { useRef, useState } from "react";
import { Icon, PillButton, Segmented } from "@/components/common";
import {
  useConfigureMCPServer,
  useRemoveMCPServer,
  useTestMCPServer,
} from "@/lib/agent/useMCPServerConfig";
import { cn } from "@/lib/utils";
import { useT } from "@/lib/i18n";
import { ToolControls } from "./ToolControls";

type Probe = { state: "idle" | "busy" } | { state: "ok" } | { state: "error"; reason: string };

const FIELD =
  "h-8 w-full rounded-md border border-line-soft bg-surface px-2.5 font-mono text-[12px] text-fg outline-none placeholder:text-fg-faint focus:border-accent focus:shadow-[0_0_0_3px_color-mix(in_srgb,var(--color-accent)_14%,transparent)]";
const AREA =
  "w-full resize-y rounded-md border border-line-soft bg-surface px-2.5 py-1.5 font-mono text-[12px] leading-[1.5] text-fg outline-none placeholder:text-fg-faint focus:border-accent focus:shadow-[0_0_0_3px_color-mix(in_srgb,var(--color-accent)_14%,transparent)]";

function linesToList(text: string): string[] | undefined {
  const list = text
    .split("\n")
    .map((l) => l.trim())
    .filter(Boolean);
  return list.length ? list : undefined;
}

interface Props {
  /** Existing server to edit, or undefined for the "add server" form. */
  server?: MCPServerConfigInfo;
  /** Called after a successful save / delete so the parent can collapse. */
  onDone: () => void;
  onCancel: () => void;
}

export function ServerForm({ server, onDone, onCancel }: Props) {
  const t = useT();
  const configure = useConfigureMCPServer();
  const remove = useRemoveMCPServer();
  const test = useTestMCPServer();
  const isEdit = server !== undefined;

  const [name, setName] = useState(server?.name ?? "");
  const [transport, setTransport] = useState<MCPTransport>(server?.transport ?? "stdio");
  const [description, setDescription] = useState(server?.description ?? "");
  // stdio
  const [command, setCommand] = useState(server?.command ?? "");
  const [args, setArgs] = useState((server?.args ?? []).join("\n"));
  const [env, setEnv] = useState((server?.env ?? []).join("\n"));
  const [dir, setDir] = useState(server?.dir ?? "");
  // http
  const [url, setUrl] = useState(server?.url ?? "");
  const [authorization, setAuthorization] = useState("");
  // Per-tool gating (edited via ToolControls for an existing server). Sparse:
  // an absent tool = enabled + not auto-approved.
  const [disabledTools, setDisabledTools] = useState<string[]>(server?.disabledTools ?? []);
  const [autoApproveTools, setAutoApproveTools] = useState<string[]>(
    server?.autoApproveTools ?? [],
  );

  const [saving, setSaving] = useState(false);
  const [probe, setProbe] = useState<Probe>({ state: "idle" });
  // Same monotonic guard as the providers pane: a Test against an old field set
  // can resolve after a Save reset the form — the token discards stale results.
  const probeSeq = useRef(0);

  const hasAuthStored = (server?.authorizationMasked ?? "") !== "";

  const buildRequest = (): ConfigureMCPServerRequest => {
    const base: ConfigureMCPServerRequest = {
      name: name.trim(),
      transport,
      enabled: server?.enabled ?? true,
      description: description.trim() || undefined,
      // Per-tool gating, edited below via ToolControls (existing server only).
      // Sparse — omit empty lists so the wire carries only non-default entries.
      disabledTools: disabledTools.length ? disabledTools : undefined,
      autoApproveTools: autoApproveTools.length ? autoApproveTools : undefined,
    };
    if (transport === "stdio") {
      return {
        ...base,
        command: command.trim() || undefined,
        args: linesToList(args),
        env: linesToList(env),
        dir: dir.trim() || undefined,
      };
    }
    return {
      ...base,
      url: url.trim() || undefined,
      // Empty = keep the stored token (the runtime treats omitted as "keep").
      authorization: authorization.trim() || undefined,
    };
  };

  const valid =
    name.trim() !== "" && (transport === "stdio" ? command.trim() !== "" : url.trim() !== "");

  const onSave = async () => {
    setSaving(true);
    probeSeq.current++;
    setProbe({ state: "idle" });
    try {
      await configure(buildRequest());
      onDone();
    } catch (err) {
      setProbe({
        state: "error",
        reason: err instanceof Error ? err.message : t("mcp.error.save"),
      });
    } finally {
      setSaving(false);
    }
  };

  const onTest = async () => {
    const token = ++probeSeq.current;
    setProbe({ state: "busy" });
    try {
      const r = await test(buildRequest());
      if (probeSeq.current !== token) return;
      setProbe(r.ok ? { state: "ok" } : { state: "error", reason: r.error ?? t("mcp.error.test") });
    } catch (err) {
      if (probeSeq.current !== token) return;
      setProbe({
        state: "error",
        reason: err instanceof Error ? err.message : t("mcp.error.test"),
      });
    }
  };

  const onDelete = async () => {
    if (!server) return;
    setSaving(true);
    try {
      await remove(server.name);
      onDone();
    } catch (err) {
      setProbe({
        state: "error",
        reason: err instanceof Error ? err.message : t("mcp.error.remove"),
      });
      setSaving(false);
    }
  };

  return (
    <div className="flex flex-col gap-2.5 rounded-md bg-surface-2 p-3">
      <div className="grid grid-cols-[minmax(0,1fr)_auto] items-center gap-3">
        <input
          type="text"
          aria-label={t("mcp.form.name.aria")}
          value={name}
          disabled={isEdit}
          onChange={(e) => setName(e.target.value)}
          placeholder={t("mcp.form.name.placeholder")}
          className={cn(FIELD, isEdit && "cursor-not-allowed opacity-60")}
        />
        <Segmented<MCPTransport>
          value={transport}
          options={[
            { value: "stdio", label: t("mcp.transport.stdio") },
            { value: "http", label: t("mcp.transport.http") },
          ]}
          onChange={setTransport}
          ariaLabel={t("mcp.form.transport.aria")}
        />
      </div>

      {transport === "stdio" ? (
        <>
          <input
            type="text"
            aria-label={t("mcp.form.command.aria")}
            value={command}
            onChange={(e) => setCommand(e.target.value)}
            placeholder={t("mcp.form.command.placeholder")}
            className={FIELD}
          />
          <label className="flex flex-col gap-1 text-[11px] text-fg-muted">
            {t("mcp.form.args")}
            <textarea
              value={args}
              onChange={(e) => setArgs(e.target.value)}
              rows={2}
              aria-label={t("mcp.form.args")}
              placeholder={t("mcp.form.args.placeholder")}
              className={AREA}
            />
          </label>
          <label className="flex flex-col gap-1 text-[11px] text-fg-muted">
            {t("mcp.form.env")}
            <textarea
              value={env}
              onChange={(e) => setEnv(e.target.value)}
              rows={2}
              aria-label={t("mcp.form.env")}
              placeholder={t("mcp.form.env.placeholder")}
              className={AREA}
            />
          </label>
          <input
            type="text"
            aria-label={t("mcp.form.dir.aria")}
            value={dir}
            onChange={(e) => setDir(e.target.value)}
            placeholder={t("mcp.form.dir.placeholder")}
            className={FIELD}
          />
        </>
      ) : (
        <>
          <input
            type="text"
            aria-label={t("mcp.form.url.aria")}
            value={url}
            onChange={(e) => setUrl(e.target.value)}
            placeholder={t("mcp.form.url.placeholder")}
            className={FIELD}
          />
          <input
            type="password"
            aria-label={t("mcp.form.auth.aria")}
            value={authorization}
            onChange={(e) => setAuthorization(e.target.value)}
            placeholder={hasAuthStored ? t("mcp.form.auth.keep") : t("mcp.form.auth.placeholder")}
            className={FIELD}
          />
        </>
      )}

      <input
        type="text"
        aria-label={t("mcp.form.description.aria")}
        value={description}
        onChange={(e) => setDescription(e.target.value)}
        placeholder={t("mcp.form.description.placeholder")}
        className={FIELD}
      />

      {server && (
        <div className="flex flex-col gap-1.5">
          <span className="text-[11px] text-fg-muted">{t("mcp.tools.manage")}</span>
          <ToolControls
            server={server.name}
            disabledTools={disabledTools}
            autoApproveTools={autoApproveTools}
            onChange={(next) => {
              setDisabledTools(next.disabledTools);
              setAutoApproveTools(next.autoApproveTools);
            }}
          />
        </div>
      )}

      <div className="flex flex-wrap items-center gap-2">
        <PillButton
          variant="accent"
          size="sm"
          disabled={!valid || saving}
          onClick={() => void onSave()}
        >
          {saving ? t("mcp.saving") : isEdit ? t("mcp.save") : t("mcp.add")}
        </PillButton>
        <PillButton
          variant="outlined"
          size="sm"
          disabled={!valid || probe.state === "busy"}
          onClick={() => void onTest()}
        >
          {probe.state === "busy" ? t("mcp.testing") : t("mcp.test")}
        </PillButton>
        <PillButton variant="outlined" size="sm" onClick={onCancel}>
          {t("common.cancel")}
        </PillButton>
        {isEdit && (
          <PillButton variant="danger" size="sm" disabled={saving} onClick={() => void onDelete()}>
            {t("mcp.delete")}
          </PillButton>
        )}

        {probe.state === "ok" && (
          <span className="inline-flex items-center gap-1 text-[12px] text-success">
            <Icon name="check" size={13} /> {t("mcp.connectionOk")}
          </span>
        )}
        {probe.state === "error" && (
          <span className="inline-flex min-w-0 items-center gap-1 text-[12px] text-negative">
            <Icon name="alert" size={13} />
            <span className="truncate" title={probe.reason}>
              {probe.reason}
            </span>
          </span>
        )}
      </div>
    </div>
  );
}
