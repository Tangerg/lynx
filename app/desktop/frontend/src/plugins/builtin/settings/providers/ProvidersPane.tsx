import { DataView } from "@/components/common";
import { useProviders } from "@/lib/data/queries";
import { useT } from "@/lib/i18n";
import { ProviderRow } from "./ProviderRow";
import { EmbeddingModelSection, UtilityModelSection } from "./RoleSections";

export function ProvidersPane() {
  const t = useT();
  const { data, isLoading, isError } = useProviders();

  return (
    <div className="flex flex-col gap-3">
      <UtilityModelSection />
      <EmbeddingModelSection />
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
          <div className="flex flex-col gap-2">
            {rows.map((p) => (
              <ProviderRow key={p.id} p={p} />
            ))}
          </div>
        )}
      </DataView>
    </div>
  );
}
