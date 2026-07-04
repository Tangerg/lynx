import { useState } from "react";
import { Icon, PillButton, Segmented } from "@/ui";
import {
  type MCPServerConfig,
  type MCPServerTransport,
  useConfigureMCPServer,
  useRemoveMCPServer,
  useTestMCPServer,
} from "../application/mcpServerConfig";
import { cn } from "@/lib/utils";
import { useT } from "@/lib/i18n";
import { FIELD, LinesField } from "./ServerFormFields";
import {
  type MCPServerDraft,
  initialMCPServerDraft,
  isMCPServerDraftValid,
  mcpServerInputFromDraft,
} from "../application/mcpServerDraft";
import { ToolControls } from "./ToolControls";
import { useProbe } from "../../useProbe";

interface Props {
  server?: MCPServerConfig;
  onDone: () => void;
  onCancel: () => void;
}

export function ServerForm({ server, onDone, onCancel }: Props) {
  const t = useT();
  const configure = useConfigureMCPServer();
  const remove = useRemoveMCPServer();
  const test = useTestMCPServer();
  const isEdit = server !== undefined;

  const [draft, setDraft] = useState<MCPServerDraft>(() => initialMCPServerDraft(server));

  const [saving, setSaving] = useState(false);
  const { probe, reset, fail, run } = useProbe();

  const hasAuthStored = (server?.authorizationMasked ?? "") !== "";

  const updateDraft = <K extends keyof MCPServerDraft>(key: K, value: MCPServerDraft[K]) => {
    setDraft((current) => ({ ...current, [key]: value }));
  };

  const buildInput = () => mcpServerInputFromDraft(draft, server);

  const valid = isMCPServerDraftValid(draft);

  const onSave = async () => {
    setSaving(true);
    reset(); // invalidate any in-flight test so its result can't overwrite this save
    try {
      await configure(buildInput());
      onDone();
    } catch (err) {
      fail(err instanceof Error ? err.message : t("mcp.error.save"));
    } finally {
      setSaving(false);
    }
  };

  const onTest = () => run(() => test(buildInput()), t("mcp.error.test"));

  const onDelete = async () => {
    if (!server) return;
    setSaving(true);
    try {
      await remove(server.name);
      onDone();
    } catch (err) {
      fail(err instanceof Error ? err.message : t("mcp.error.remove"));
      setSaving(false);
    }
  };

  return (
    <div className="flex flex-col gap-2.5 rounded-md bg-surface-2 p-3">
      <div className="grid grid-cols-[minmax(0,1fr)_auto] items-center gap-3">
        <input
          type="text"
          aria-label={t("mcp.form.name.aria")}
          value={draft.name}
          disabled={isEdit}
          onChange={(e) => updateDraft("name", e.target.value)}
          placeholder={t("mcp.form.name.placeholder")}
          className={cn(FIELD, isEdit && "cursor-not-allowed opacity-60")}
        />
        <Segmented<MCPServerTransport>
          value={draft.transport}
          options={[
            { value: "stdio", label: t("mcp.transport.stdio") },
            { value: "streamableHttp", label: t("mcp.transport.http") },
          ]}
          onChange={(value) => updateDraft("transport", value)}
          ariaLabel={t("mcp.form.transport.aria")}
        />
      </div>

      {draft.transport === "stdio" ? (
        <>
          <input
            type="text"
            aria-label={t("mcp.form.command.aria")}
            value={draft.command}
            onChange={(e) => updateDraft("command", e.target.value)}
            placeholder={t("mcp.form.command.placeholder")}
            className={FIELD}
          />
          <LinesField
            label={t("mcp.form.args")}
            value={draft.args}
            onChange={(value) => updateDraft("args", value)}
            placeholder={t("mcp.form.args.placeholder")}
          />
          <LinesField
            label={t("mcp.form.env")}
            value={draft.env}
            onChange={(value) => updateDraft("env", value)}
            placeholder={t("mcp.form.env.placeholder")}
          />
          <input
            type="text"
            aria-label={t("mcp.form.dir.aria")}
            value={draft.dir}
            onChange={(e) => updateDraft("dir", e.target.value)}
            placeholder={t("mcp.form.dir.placeholder")}
            className={FIELD}
          />
        </>
      ) : (
        <>
          <input
            type="text"
            aria-label={t("mcp.form.url.aria")}
            value={draft.url}
            onChange={(e) => updateDraft("url", e.target.value)}
            placeholder={t("mcp.form.url.placeholder")}
            className={FIELD}
          />
          <input
            type="password"
            aria-label={t("mcp.form.auth.aria")}
            value={draft.authorization}
            onChange={(e) => updateDraft("authorization", e.target.value)}
            placeholder={hasAuthStored ? t("mcp.form.auth.keep") : t("mcp.form.auth.placeholder")}
            className={FIELD}
          />
          <LinesField
            label={t("mcp.form.headers")}
            value={draft.headers}
            onChange={(value) => updateDraft("headers", value)}
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
          value={draft.timeoutSec}
          onChange={(e) => updateDraft("timeoutSec", e.target.value)}
          placeholder={t("mcp.form.timeout.placeholder")}
          className={cn(FIELD, "tabular-nums")}
        />
      </label>

      <input
        type="text"
        aria-label={t("mcp.form.description.aria")}
        value={draft.description}
        onChange={(e) => updateDraft("description", e.target.value)}
        placeholder={t("mcp.form.description.placeholder")}
        className={FIELD}
      />

      {server && (
        <div className="flex flex-col gap-1.5">
          <span className="text-[11px] text-fg-muted">{t("mcp.tools.manage")}</span>
          <ToolControls
            server={server.name}
            disabledTools={draft.disabledTools}
            autoApproveTools={draft.autoApproveTools}
            onChange={(next) => {
              setDraft((current) => ({ ...current, ...next }));
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
