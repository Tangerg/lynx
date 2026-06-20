// Add / edit form for one MCP server. Transport segmented control switches the
// dynamic field set: stdio = command + args (one per line) + env (KEY=value per
// line) + dir; http = url + authorization (password, "leave blank to keep") +
// headers (KEY=value per line). A shared timeout (seconds) applies to both.
// Save → configure; Test → live probe with an inline ok/err chip; Delete on an
// existing server. Mirrors the providers pane's save/test/probe-token flow.

import type { MCPServerConfigInfo, MCPTransport } from "@/lib/data/queries";
import type { ConfigureMCPServerRequest } from "@/rpc";
import { useRef, useState } from "react";
import { Icon, INPUT_FOCUS_RING, PillButton, Segmented } from "@/components/common";
import {
  useConfigureMCPServer,
  useRemoveMCPServer,
  useTestMCPServer,
} from "@/lib/agent/useMCPServerConfig";
import { cn } from "@/lib/utils";
import { useT } from "@/lib/i18n";
import { ToolControls } from "./ToolControls";

type Probe = { state: "idle" | "busy" } | { state: "ok" } | { state: "error"; reason: string };

const FIELD = cn(
  "h-8 w-full rounded-md border border-line-soft bg-surface px-2.5 font-mono text-[12px] text-fg outline-none placeholder:text-fg-faint",
  INPUT_FOCUS_RING,
);
const AREA = cn(
  "w-full resize-y rounded-md border border-line-soft bg-surface px-2.5 py-1.5 font-mono text-[12px] leading-[1.5] text-fg outline-none placeholder:text-fg-faint",
  INPUT_FOCUS_RING,
);

function linesToList(text: string): string[] | undefined {
  const list = text
    .split("\n")
    .map((l) => l.trim())
    .filter(Boolean);
  return list.length ? list : undefined;
}

// linesToMap parses "KEY=value" lines (env / headers) into a map, splitting on
// the FIRST '=' so a value may contain '='. Blank lines are skipped; a line with
// no '=' becomes a bare key. Empty → undefined (omit from the wire).
function linesToMap(text: string): Record<string, string> | undefined {
  const out: Record<string, string> = {};
  for (const raw of text.split("\n")) {
    const line = raw.trim();
    if (!line) continue;
    const i = line.indexOf("=");
    if (i === -1) out[line] = "";
    else out[line.slice(0, i)] = line.slice(i + 1);
  }
  return Object.keys(out).length ? out : undefined;
}

// mapToLines renders a KEY→value map back to "KEY=value" lines for the editor.
function mapToLines(m: Record<string, string> | undefined): string {
  return m
    ? Object.entries(m)
        .map(([k, v]) => `${k}=${v}`)
        .join("\n")
    : "";
}

// LinesField is a labeled multi-line text field — the shared shape of the args /
// env / headers editors (one entry per line). The label doubles as the aria
// label since each is self-describing.
function LinesField({
  label,
  value,
  onChange,
  placeholder,
}: {
  label: string;
  value: string;
  onChange: (value: string) => void;
  placeholder: string;
}) {
  return (
    <label className="flex flex-col gap-1 text-[11px] text-fg-muted">
      {label}
      <textarea
        value={value}
        onChange={(e) => onChange(e.target.value)}
        rows={2}
        aria-label={label}
        placeholder={placeholder}
        className={AREA}
      />
    </label>
  );
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
  const [transport, setTransport] = useState<MCPTransport>(server?.type ?? "stdio");
  const [description, setDescription] = useState(server?.description ?? "");
  // stdio
  const [command, setCommand] = useState(server?.command ?? "");
  const [args, setArgs] = useState((server?.args ?? []).join("\n"));
  const [env, setEnv] = useState(mapToLines(server?.env));
  const [dir, setDir] = useState(server?.dir ?? "");
  // http
  const [url, setUrl] = useState(server?.url ?? "");
  const [authorization, setAuthorization] = useState("");
  const [headers, setHeaders] = useState(mapToLines(server?.headers));
  // Connection-handshake timeout in seconds (both transports); "" = unbounded.
  // Named *Sec to avoid shadowing the global setTimeout.
  const [timeoutSec, setTimeoutSec] = useState(
    server?.timeoutSeconds ? String(server.timeoutSeconds) : "",
  );
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
    const secs = parseInt(timeoutSec, 10);
    const base: ConfigureMCPServerRequest = {
      name: name.trim(),
      type: transport,
      enabled: server?.enabled ?? true,
      description: description.trim() || undefined,
      timeoutSeconds: Number.isFinite(secs) && secs > 0 ? secs : undefined,
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
        env: linesToMap(env),
        dir: dir.trim() || undefined,
      };
    }
    return {
      ...base,
      url: url.trim() || undefined,
      // Empty = keep the stored token (the runtime treats omitted as "keep").
      authorization: authorization.trim() || undefined,
      headers: linesToMap(headers),
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
            { value: "streamableHttp", label: t("mcp.transport.http") },
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
          <LinesField
            label={t("mcp.form.args")}
            value={args}
            onChange={setArgs}
            placeholder={t("mcp.form.args.placeholder")}
          />
          <LinesField
            label={t("mcp.form.env")}
            value={env}
            onChange={setEnv}
            placeholder={t("mcp.form.env.placeholder")}
          />
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
          <LinesField
            label={t("mcp.form.headers")}
            value={headers}
            onChange={setHeaders}
            placeholder={t("mcp.form.headers.placeholder")}
          />
        </>
      )}

      <label className="flex flex-col gap-1 text-[11px] text-fg-muted">
        {t("mcp.form.timeout")}
        <input
          type="number"
          min={0}
          inputMode="numeric"
          aria-label={t("mcp.form.timeout")}
          value={timeoutSec}
          onChange={(e) => setTimeoutSec(e.target.value)}
          placeholder={t("mcp.form.timeout.placeholder")}
          className={cn(FIELD, "tabular-nums")}
        />
      </label>

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
