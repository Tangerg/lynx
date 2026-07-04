import { useState } from "react";
import { DataView, Icon, PillButton } from "@/ui";
import { cn } from "@/lib/utils";
import { useT } from "@/lib/i18n";
import { useMCPServerConfigs } from "../application/mcpServerConfig";
import { JsonImport } from "./JsonImport";
import { ServerForm } from "./ServerForm";
import { ServerRow } from "./ServerRow";

export function McpServersPane() {
  const t = useT();
  const { data, isLoading, isError } = useMCPServerConfigs();
  const [adding, setAdding] = useState(false);

  return (
    <div className="flex flex-col gap-3">
      <div className={cn("flex items-center justify-between gap-3", adding && "items-start")}>
        {adding ? (
          <div className="flex-1">
            <ServerForm onDone={() => setAdding(false)} onCancel={() => setAdding(false)} />
          </div>
        ) : (
          <>
            <JsonImport />
            <PillButton variant="outlined" size="sm" onClick={() => setAdding(true)}>
              <Icon name="plus" size={13} />
              {t("mcp.add")}
            </PillButton>
          </>
        )}
      </div>

      <DataView
        items={data}
        isLoading={isLoading}
        isError={isError}
        skeletonCount={3}
        empty={{
          icon: "tool",
          title: t("mcp.empty"),
          sub: t("mcp.empty.sub"),
        }}
      >
        {(rows) => (
          <div className="flex flex-col gap-2">
            {rows.map((s) => (
              <ServerRow key={s.name} server={s} />
            ))}
          </div>
        )}
      </DataView>
    </div>
  );
}
