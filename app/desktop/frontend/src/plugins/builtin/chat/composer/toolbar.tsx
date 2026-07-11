import { useEffect, useRef } from "react";
import { AgentIconButton } from "@/ui/agent";
import { DropdownMenu, Icon, ProviderIcon, StatusDot, Tooltip } from "@/ui";
import { imageFiles } from "@/plugins/builtin/chat/composer/public/input";
import { useSelectedModel } from "./public/selectedModel";
import {
  APPROVAL_MODES,
  DEFAULT_APPROVAL_MODE,
  setApprovalMode,
  useApprovalMode,
  type ApprovalModeValue,
} from "@/plugins/builtin/agent/public/approvalPolicy";
import { useModels } from "@/plugins/builtin/settings/providers/public/data";
import { rpcErrorText } from "@/lib/rpcErrors";
import { notifyError } from "@/lib/notify";
import { useT } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import { definePlugin } from "@/plugins/sdk";
import {
  composerApprovalSlot,
  composerAttachSlot,
  composerModelSlot,
} from "./application/composerContributions";
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
        className="inline-flex h-8 shrink-0 items-center gap-1.5 rounded-md px-2.5 opacity-60"
        aria-hidden
      >
        <span className="h-1.5 w-1.5 rounded-full bg-surface-2" />
        <span className="h-3 w-16 rounded-sm bg-surface-2" />
      </div>
    );
  }
  const selected = models.find((m) => m.provider === provider && m.id === model) ?? models[0]!;

  return (
    <DropdownMenu.Root>
      <DropdownMenu.Trigger
        render={
          <button
            type="button"
            aria-label={t("composer.switchModel")}
            className="inline-flex h-8 shrink-0 items-center gap-1.5 whitespace-nowrap rounded-md px-2.5 font-sans text-[13px] font-medium text-fg-soft transition-colors hover:bg-fg/[0.05] hover:text-fg data-[popup-open]:bg-fg/[0.05] data-[popup-open]:text-fg"
            data-slot="composer-model"
          >
            <StatusDot tone="idle" />
            <span className="max-w-[168px] truncate">{selected.label}</span>
            <Icon name="chevron-down" size={14} className="shrink-0 text-fg-faint" />
          </button>
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
          icon="plus"
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

// Approval-mode pill — the composer's primary access control (Codex "完全访问").
// A ghost pill that turns warning-toned when full access ("yolo") is on.
function ApprovalModePill() {
  const t = useT();
  const { data: mode, isError } = useApprovalMode();
  if (isError || mode === undefined) return null;
  const current = APPROVAL_MODES.find((m) => m.value === mode) ?? DEFAULT_APPROVAL_MODE;
  const full = mode === "yolo";
  const onSelect = async (next: ApprovalModeValue) => {
    if (next === mode) return;
    try {
      await setApprovalMode(next);
    } catch (err) {
      notifyError(rpcErrorText(err) ?? t("approvals.error.mode"));
    }
  };
  return (
    <DropdownMenu.Root>
      <DropdownMenu.Trigger
        render={
          <button
            type="button"
            aria-label={t("approvals.mode.aria")}
            className={cn(
              "inline-flex h-8 shrink-0 items-center gap-1.5 whitespace-nowrap rounded-md px-2.5 font-sans text-[13px] font-medium transition-colors data-[popup-open]:bg-fg/[0.05]",
              full
                ? "text-warning hover:bg-warning/10"
                : "text-fg-soft hover:bg-fg/[0.05] hover:text-fg",
            )}
            data-slot="composer-approval"
          >
            <Icon name={full ? "alert" : "shield"} size={14} className="shrink-0" />
            <span className="max-w-[132px] truncate">{t(current.labelKey)}</span>
            <Icon name="chevron-down" size={14} className="shrink-0 text-fg-faint" />
          </button>
        }
      />
      <DropdownMenu.Content align="start" sideOffset={6} className="min-w-[248px]">
        {APPROVAL_MODES.map((m) => (
          <DropdownMenu.Item
            key={m.value}
            onClick={() => void onSelect(m.value)}
            className="grid grid-cols-[minmax(0,1fr)_14px] items-start gap-2 rounded-md px-2 py-1.5 outline-none data-[highlighted]:bg-fg/[0.06]"
          >
            <span className="min-w-0">
              <span className="block text-[12.5px] font-semibold text-fg">{t(m.labelKey)}</span>
              <span className="block text-[11.5px] leading-snug text-fg-muted">{t(m.descKey)}</span>
            </span>
            {m.value === mode && <Icon name="check" size={12} className="mt-0.5 text-accent" />}
          </DropdownMenu.Item>
        ))}
      </DropdownMenu.Content>
    </DropdownMenu.Root>
  );
}

export const composerToolbar = definePlugin({
  name: "lyra.builtin.composer-toolbar",
  version: "1.0.0",
  setup({ host }) {
    host.layout.register("composer.toolbar.start", composerAttachSlot(AttachButton));
    host.layout.register("composer.toolbar.start", composerApprovalSlot(ApprovalModePill));
    host.layout.register("composer.toolbar.start", composerModelSlot(ModelPicker));
  },
});
