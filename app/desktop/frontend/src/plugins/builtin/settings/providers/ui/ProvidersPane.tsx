import { DataView } from "@/ui";
import { useT } from "@/lib/i18n";
import { useProviderConfigs } from "../application/providerConfig";
import { ProviderRow } from "./ProviderRow";
import { EmbeddingModelSection, UtilityModelSection } from "./RoleSections";

export function ProvidersPane() {
  const t = useT();
  const { data, isLoading, isError } = useProviderConfigs();

  return (
    <div className="flex flex-col gap-6">
      <div className="flex flex-col gap-3">
        <UtilityModelSection />
        <EmbeddingModelSection />
      </div>
      <DataView
        items={data}
        isLoading={isLoading}
        isError={isError}
        skeletonCount={3}
        empty={{
          icon: "spark",
          title: t("providers.empty"),
          sub: t("providers.empty.sub"),
        }}
      >
        {(rows) => (
          <div className="flex flex-col gap-1 rounded-[14px] bg-surface p-2">
            {rows.map((p) => (
              <ProviderRow key={p.id} p={p} />
            ))}
          </div>
        )}
      </DataView>
    </div>
  );
}
