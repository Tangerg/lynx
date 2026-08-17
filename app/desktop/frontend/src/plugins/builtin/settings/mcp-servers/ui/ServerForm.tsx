import { useState } from "react";
import { Icon, PillButton, Segmented, Surface, Switch, TextField } from "@/ui";
import {
  type MCPServerSettings,
  type MCPTransport,
  mcpServerMutationWasRetired,
  useMCPServerMutationMaterialGeneration,
  useCreateMCPServer,
  useDeleteMCPServer,
  useTestMCPServer,
  useUpdateMCPServer,
} from "../application/mcpServerConfig";
import { useT } from "@/lib/i18n";
import { LinesField } from "./ServerFormFields";
import {
  editRetainedValue,
  type MCPServerDraft,
  initialMCPServerDraft,
  isMCPServerDraftValid,
  mcpAuthorizationNeedsDisposition,
  mcpEnvironmentNeedsDisposition,
  mcpHeadersNeedDisposition,
  mcpServerInputFromDraft,
  retainedValueText,
  setRetainedValueCleared,
} from "../application/mcpServerDraft";
import { ToolControls } from "./ToolControls";
import { useAsyncFeedback } from "../../public";

interface Props {
  server?: MCPServerSettings;
  onDone: () => void;
  onCancel: () => void;
}

export function ServerForm({ server, onDone, onCancel }: Props) {
  const t = useT();
  const create = useCreateMCPServer();
  const update = useUpdateMCPServer();
  const remove = useDeleteMCPServer();
  const test = useTestMCPServer();
  const isEdit = server !== undefined;

  const [draft, setDraft] = useState<MCPServerDraft>(() => initialMCPServerDraft(server));

  const [saving, setSaving] = useState(false);
  const materialGeneration = useMCPServerMutationMaterialGeneration();
  const { feedback, reset, fail, run } = useAsyncFeedback(materialGeneration);

  const hasAuthStored = (server?.authorizationMasked ?? "") !== "";
  const hasHeadersStored = Object.keys(server?.headersMasked ?? {}).length > 0;
  const hasEnvironmentStored = Object.keys(server?.envMasked ?? {}).length > 0;

  const updateDraft = <K extends keyof MCPServerDraft>(key: K, value: MCPServerDraft[K]) => {
    setDraft((current) => ({ ...current, [key]: value }));
  };

  const buildInput = () => mcpServerInputFromDraft(draft, server);

  const needsAuthorizationDisposition = mcpAuthorizationNeedsDisposition(draft, server);
  const needsHeadersDisposition = mcpHeadersNeedDisposition(draft, server);
  const needsEnvironmentDisposition = mcpEnvironmentNeedsDisposition(draft, server);
  const valid = isMCPServerDraftValid(draft, server);

  const onSave = async () => {
    setSaving(true);
    reset(); // invalidate any in-flight test so its result can't overwrite this save
    try {
      const input = buildInput();
      if (server) await update(server.name, input);
      else await create(input);
      onDone();
    } catch (err) {
      if (mcpServerMutationWasRetired(err)) return;
      fail(err instanceof Error ? err.message : t("mcp.error.save"));
    } finally {
      setSaving(false);
    }
  };

  const onTest = () =>
    run(() => test(buildInput()), t("mcp.error.test"), mcpServerMutationWasRetired);

  const onDelete = async () => {
    if (!server) return;
    setSaving(true);
    try {
      await remove(server.name);
      onDone();
    } catch (err) {
      if (mcpServerMutationWasRetired(err)) return;
      fail(err instanceof Error ? err.message : t("mcp.error.remove"));
    } finally {
      setSaving(false);
    }
  };

  return (
    <Surface className="flex flex-col gap-3">
      <div className="grid grid-cols-[minmax(0,1fr)_auto] items-center gap-3">
        <TextField
          type="text"
          aria-label={t("mcp.form.name.aria")}
          value={draft.name}
          disabled={isEdit}
          onChange={(e) => updateDraft("name", e.target.value)}
          placeholder={t("mcp.form.name.placeholder")}
        />
        <Segmented<MCPTransport>
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
          <TextField
            type="text"
            aria-label={t("mcp.form.command.aria")}
            value={draft.command}
            onChange={(e) => updateDraft("command", e.target.value)}
            placeholder={t("mcp.form.command.placeholder")}
          />
          <LinesField
            label={t("mcp.form.args")}
            value={draft.args}
            onChange={(value) => updateDraft("args", value)}
            placeholder={t("mcp.form.args.placeholder")}
          />
          <LinesField
            label={t("mcp.form.env")}
            value={retainedValueText(draft.environment)}
            onChange={(value) => updateDraft("environment", editRetainedValue(value))}
            placeholder={
              hasEnvironmentStored ? t("mcp.form.env.keep") : t("mcp.form.env.placeholder")
            }
          />
          {hasEnvironmentStored && (
            <label className="flex items-center justify-between gap-3 text-ui-md text-fg-muted">
              <span>{t("mcp.form.env.clear")}</span>
              <Switch
                checked={draft.environment.disposition === "clear"}
                onCheckedChange={(value) =>
                  updateDraft("environment", setRetainedValueCleared(value))
                }
                ariaLabel={t("mcp.form.env.clear")}
              />
            </label>
          )}
          {needsEnvironmentDisposition && (
            <span className="text-ui-md text-warning">{t("mcp.form.env.targetChanged")}</span>
          )}
          <TextField
            type="text"
            aria-label={t("mcp.form.dir.aria")}
            value={draft.dir}
            onChange={(e) => updateDraft("dir", e.target.value)}
            placeholder={t("mcp.form.dir.placeholder")}
          />
        </>
      ) : (
        <>
          <TextField
            type="text"
            aria-label={t("mcp.form.url.aria")}
            value={draft.url}
            onChange={(e) => updateDraft("url", e.target.value)}
            placeholder={t("mcp.form.url.placeholder")}
          />
          <TextField
            type="password"
            aria-label={t("mcp.form.auth.aria")}
            value={retainedValueText(draft.authorization)}
            onChange={(e) => updateDraft("authorization", editRetainedValue(e.target.value))}
            placeholder={hasAuthStored ? t("mcp.form.auth.keep") : t("mcp.form.auth.placeholder")}
          />
          {hasAuthStored && (
            <label className="flex items-center justify-between gap-3 text-ui-md text-fg-muted">
              <span>{t("mcp.form.auth.clear")}</span>
              <Switch
                checked={draft.authorization.disposition === "clear"}
                onCheckedChange={(value) =>
                  updateDraft("authorization", setRetainedValueCleared(value))
                }
                ariaLabel={t("mcp.form.auth.clear")}
              />
            </label>
          )}
          {needsAuthorizationDisposition && (
            <span className="text-ui-md text-warning">{t("mcp.form.auth.originChanged")}</span>
          )}
          <LinesField
            label={t("mcp.form.headers")}
            value={retainedValueText(draft.headers)}
            onChange={(value) => updateDraft("headers", editRetainedValue(value))}
            placeholder={
              hasHeadersStored ? t("mcp.form.headers.keep") : t("mcp.form.headers.placeholder")
            }
          />
          {hasHeadersStored && (
            <label className="flex items-center justify-between gap-3 text-ui-md text-fg-muted">
              <span>{t("mcp.form.headers.clear")}</span>
              <Switch
                checked={draft.headers.disposition === "clear"}
                onCheckedChange={(value) => updateDraft("headers", setRetainedValueCleared(value))}
                ariaLabel={t("mcp.form.headers.clear")}
              />
            </label>
          )}
          {needsHeadersDisposition && (
            <span className="text-ui-md text-warning">{t("mcp.form.headers.originChanged")}</span>
          )}
        </>
      )}

      <label className="flex flex-col gap-1.5">
        <span className="text-ui-md font-medium text-fg">{t("mcp.form.timeout")}</span>
        <TextField
          type="number"
          min={0}
          inputMode="numeric"
          aria-label={t("mcp.form.timeout")}
          value={draft.timeoutSec}
          onChange={(e) => updateDraft("timeoutSec", e.target.value)}
          placeholder={t("mcp.form.timeout.placeholder")}
          className="tabular-nums"
        />
      </label>

      <TextField
        type="text"
        aria-label={t("mcp.form.description.aria")}
        value={draft.description}
        onChange={(e) => updateDraft("description", e.target.value)}
        placeholder={t("mcp.form.description.placeholder")}
      />

      {server && (
        <div className="flex flex-col gap-1.5">
          <span className="text-ui-md font-medium text-fg">{t("mcp.tools.manage")}</span>
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
          disabled={!valid || feedback.state === "busy"}
          onClick={() => void onTest()}
        >
          {feedback.state === "busy" ? t("mcp.testing") : t("mcp.test")}
        </PillButton>
        <PillButton variant="outlined" size="sm" onClick={onCancel}>
          {t("common.cancel")}
        </PillButton>
        {isEdit && (
          <PillButton variant="danger" size="sm" disabled={saving} onClick={() => void onDelete()}>
            {t("mcp.delete")}
          </PillButton>
        )}

        {feedback.state === "ok" && (
          <span className="inline-flex items-center gap-1 text-ui-md text-success">
            <Icon name="check" size="sm" /> {t("mcp.connectionOk")}
          </span>
        )}
        {feedback.state === "error" && (
          <span className="inline-flex min-w-0 items-center gap-1 text-ui-md text-negative">
            <Icon name="alert" size="sm" />
            <span className="truncate" title={feedback.reason}>
              {feedback.reason}
            </span>
          </span>
        )}
      </div>
    </Surface>
  );
}
