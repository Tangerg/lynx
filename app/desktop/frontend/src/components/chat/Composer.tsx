import { useEffect, useMemo, useRef } from "react";
import { Chip, Icon, Segmented, type IconName } from "@/components/common";
import { Slot } from "@/plugins/Slot";
import {
  lookupComposerKeyBinding,
  normalizeCombo,
  pickComposerPlaceholder,
  useComposerAttachmentSources,
  useComposerModes,
} from "@/plugins/sdk";
import { submitComposer } from "./submitComposer";

const FALLBACK_PLACEHOLDER = "Ask, plan, or paste a stack trace…  /  to run a command";

// The mode is a free-form id — built-ins ship "agent" / "ask" / "plan" via
// the composer-modes plugin, but third-party plugins can add their own.
export type ComposerMode = string;

export type Attachment = { label: string; icon?: IconName };

type Props = {
  onSend: (text: string) => void;
  value: string;
  onChange: (v: string) => void;
  attachments: Attachment[];
  onRemoveAttachment: (i: number) => void;
  mode: ComposerMode;
  onModeChange: (m: ComposerMode) => void;
};

export function Composer({
  onSend, value, onChange, attachments, onRemoveAttachment, mode, onModeChange,
}: Props) {
  const inputRef = useRef<HTMLTextAreaElement>(null);
  const modes = useComposerModes();
  const attachmentSources = useComposerAttachmentSources();
  // Pick a placeholder once at mount — using a hook would cause the text
  // to re-roll on every render (and `pickComposerPlaceholder` is random).
  const placeholder = useMemo(() => pickComposerPlaceholder()?.text ?? FALLBACK_PLACEHOLDER, []);

  const submit = () => submitComposer({
    value,
    clear: () => onChange(""),
    sendText: onSend,
  });

  useEffect(() => {
    const el = inputRef.current;
    if (!el) return;
    el.style.height = "auto";
    el.style.height = `${Math.min(el.scrollHeight, 160)}px`;
  }, [value]);

  return (
    <div className="composer">
      <PluginAttachments sources={attachmentSources} />
      {attachments.length > 0 && (
        <div className="composer-chips">
          {attachments.map((a, i) => (
            <Chip
              key={i}
              icon={a.icon ?? "file"}
              title={a.label}
              onClose={() => onRemoveAttachment(i)}
            >
              {a.label}
            </Chip>
          ))}
        </div>
      )}
      <textarea
        ref={inputRef}
        className="composer-input"
        placeholder={placeholder}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        onKeyDown={(e) => {
          // Resolve the pressed combo to its canonical form and ask the
          // plugin registry for a binding. The built-in "composer-keymap"
          // plugin registers Enter→submit; user plugins can add more
          // (e.g. Mod+K to open a snippet drawer).
          const parts: string[] = [];
          if (e.metaKey || e.ctrlKey) parts.push("mod");
          if (e.altKey)   parts.push("alt");
          if (e.shiftKey) parts.push("shift");
          parts.push(e.key);
          const binding = lookupComposerKeyBinding(normalizeCombo(parts.join("+")));
          if (!binding) return;
          const handled = binding.handler({
            value,
            onChange,
            submit,
            event: e.nativeEvent,
          });
          if (handled) e.preventDefault();
        }}
        rows={1}
      />
      <div className="composer-toolbar">
        <Slot name="composer.toolbar.start" />
        {modes.length > 0 && (
          <Segmented
            value={mode}
            onChange={onModeChange}
            options={modes.map((m) => ({
              value: m.id,
              label: (
                <>
                  <Icon name={(m.icon as IconName) ?? "spark"} size={12} />
                  <span>{m.label}</span>
                </>
              ),
            }))}
          />
        )}
        <div className="spacer" />
        <Slot name="composer.toolbar.end" />
      </div>
    </div>
  );
}

// Render every plugin-contributed attachment source's `useAttachments()`
// output in one row. Each source's hook runs inside its own component so a
// throwing/buggy hook is isolated to that one bubble.
type AttachmentSource = ReturnType<typeof useComposerAttachmentSources>[number];
function PluginAttachments({ sources }: { sources: AttachmentSource[] }) {
  if (sources.length === 0) return null;
  return (
    <div className="composer-chips">
      {sources.map((s) => <SourceChips key={s.id} source={s} />)}
    </div>
  );
}
function SourceChips({ source }: { source: AttachmentSource }) {
  const items = source.useAttachments();
  return (
    <>
      {items.map((a, i) => (
        <Chip
          key={`${source.id}:${i}`}
          icon={(a.icon as IconName | undefined) ?? "file"}
          title={a.label}
        >
          {a.label}
        </Chip>
      ))}
    </>
  );
}

