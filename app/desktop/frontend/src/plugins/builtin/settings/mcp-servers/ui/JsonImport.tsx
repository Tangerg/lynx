import { useState } from "react";
import { Icon, PillButton, Surface, TextArea, TextButton } from "@/ui";
import { mcpServerMutationWasRetired, useCreateMCPServer } from "../application/mcpServerConfig";
import { notifyInfo } from "@/plugins/sdk";
import { useT } from "@/lib/i18n";
import { parseMcpImport } from "../application/mcpImport";

export function JsonImport() {
  const t = useT();
  const create = useCreateMCPServer();
  const [open, setOpen] = useState(false);
  const [text, setText] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | undefined>();

  const onImport = async () => {
    setBusy(true);
    setError(undefined);
    try {
      const { servers } = parseMcpImport(text);
      for (const server of servers) await create(server);
      notifyInfo(t("mcp.import.ok", { count: servers.length }), { source: "mcp" });
      setText("");
      setOpen(false);
    } catch (err) {
      if (mcpServerMutationWasRetired(err)) return;
      setError(err instanceof Error ? err.message : t("mcp.import.error"));
    } finally {
      setBusy(false);
    }
  };

  if (!open) {
    return (
      <TextButton onClick={() => setOpen(true)}>
        <Icon name="download" size="sm" />
        {t("mcp.import")}
      </TextButton>
    );
  }
  return (
    <Surface className="flex flex-col gap-2.5">
      <span className="text-ui-md text-fg-muted">{t("mcp.import.hint")}</span>
      <TextArea
        size="sm"
        invalid={error !== undefined}
        value={text}
        onChange={(event) => setText(event.target.value)}
        rows={6}
        spellCheck={false}
        aria-label={t("mcp.import.hint")}
        placeholder={
          '{"mcpServers": {"my-server": {"type": "streamableHttp", "url": "https://example.com/mcp"}}}'
        }
      />
      {error && (
        <span className="inline-flex items-center gap-1 text-ui-md text-negative">
          <Icon name="alert" size="sm" />
          <span className="truncate" title={error}>
            {error}
          </span>
        </span>
      )}
      <div className="flex items-center gap-2">
        <PillButton
          variant="accent"
          size="sm"
          disabled={!text.trim() || busy}
          onClick={() => void onImport()}
        >
          {busy ? t("mcp.importing") : t("mcp.import.confirm")}
        </PillButton>
        <PillButton variant="outlined" size="sm" onClick={() => setOpen(false)}>
          {t("common.cancel")}
        </PillButton>
      </div>
    </Surface>
  );
}
