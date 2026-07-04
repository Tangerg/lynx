import { useEffect, useRef } from "react";
import { AgentIconButton, AgentToolbarButton } from "@/components/agent-studio";
import { DropdownMenu, Icon, ProviderIcon, Tooltip } from "@/components/common";
import { imageFiles } from "@/plugins/builtin/chat/composer/public/input";
import { useSelectedModel } from "./public/selectedModel";
import { useModels } from "@/lib/data/queries";
import { useT } from "@/lib/i18n";
import { definePlugin } from "@/plugins/sdk";
import { useAddComposerImageFiles } from "./public/attachments";
import {
  useComposerModelPreference,
  useSetComposerModelPreference,
} from "./public/modelPreference";

function ModelPicker() {
  const t = useT();
  const { data: models = [], isLoading } = useModels();
  const { provider, model } = useComposerModelPreference();
  const setModel = useSetComposerModelPreference();

  useEffect(() => {
    if (!model && models.length > 0) setModel(models[0]!.provider, models[0]!.id);
  }, [model, models, setModel]);

  if (models.length === 0) {
    if (!isLoading) return null;
    return (
      <div
        className="inline-flex h-8 shrink-0 items-center gap-1.5 rounded-[8px] pl-1.5 pr-2.5 opacity-60"
        aria-hidden
      >
        <span className="h-4 w-4 rounded-full bg-surface-2" />
        <span className="h-3 w-16 rounded bg-surface-2" />
      </div>
    );
  }
  const selected = models.find((m) => m.provider === provider && m.id === model) ?? models[0]!;

  return (
    <DropdownMenu.Root>
      <DropdownMenu.Trigger
        render={
          <AgentToolbarButton
            type="button"
            aria-label={t("composer.switchModel")}
            className="gap-1.5 pl-1.5 pr-2.5 shadow-none"
            data-slot="composer-model"
          >
            <ProviderIcon provider={selected.provider} size={16} />
            <span className="font-sans text-[12.5px] font-medium">{selected.label}</span>
            <Icon name="chevron-down" size={10} className="text-fg-faint opacity-70" />
          </AgentToolbarButton>
        }
      />
      <DropdownMenu.Content align="start" sideOffset={6} className="min-w-[200px]">
        {models.map((m) => (
          <DropdownMenu.Item
            key={`${m.provider}:${m.id}`}
            onClick={() => setModel(m.provider, m.id)}
            className="grid-cols-[16px_minmax(0,1fr)_14px] px-2"
          >
            <ProviderIcon provider={m.provider} size={16} />
            <span className="truncate">{m.label}</span>
            {m.provider === selected.provider && m.id === selected.id && (
              <Icon name="check" size={12} className="text-accent" />
            )}
          </DropdownMenu.Item>
        ))}
      </DropdownMenu.Content>
    </DropdownMenu.Root>
  );
}

function AttachButton() {
  const t = useT();
  const addImageFiles = useAddComposerImageFiles();
  const inputRef = useRef<HTMLInputElement>(null);
  const canAttach = useSelectedModel()?.multimodal ?? false;

  return (
    <>
      <input
        ref={inputRef}
        type="file"
        accept="image/*"
        multiple
        aria-label={t("composer.attachImage")}
        className="hidden"
        onChange={(e) => {
          const files = imageFiles(e.target.files);
          e.target.value = "";
          if (files.length > 0) addImageFiles(files);
        }}
      />
      <Tooltip
        label={canAttach ? t("composer.attachImage") : t("composer.attachImage.unsupported")}
      >
        <AgentIconButton
          icon="image"
          aria-label={t("composer.attachImage")}
          disabled={!canAttach}
          onClick={() => inputRef.current?.click()}
          className="h-8 w-8 disabled:opacity-25"
          data-slot="composer-attach"
        />
      </Tooltip>
    </>
  );
}

export const composerToolbar = definePlugin({
  name: "lyra.builtin.composer-toolbar",
  version: "1.0.0",
  setup({ host }) {
    host.layout.register("composer.toolbar.start", {
      id: "model",
      order: 0,
      component: ModelPicker,
    });
    host.layout.register("composer.toolbar.start", {
      id: "attach",
      order: 1,
      component: AttachButton,
    });
  },
});
