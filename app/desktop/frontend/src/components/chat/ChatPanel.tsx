import { useEffect, useRef } from "react";
import { Panel } from "@/components/common";
import { InspectorProvider } from "@/components/inspector/InspectorContext";
import { PluginBoundary } from "@/plugins/PluginBoundary";
import { Slot } from "@/plugins/Slot";
import { useInspectorTabs, usePluginStore } from "@/plugins/sdk";
import { useUIStore } from "@/state/uiStore";
import type { Message, PlanItem, ToolCall } from "@/protocol/agui/viewState";
import { ChatTopBar, type ChatTab, type ViewTab } from "./ChatTopBar";
import { MessageStream } from "./MessageStream";
import { SlashSuggestions } from "./SlashSuggestions";
import { Composer, type Attachment, type ComposerMode } from "./Composer";
import { ComposerFooter } from "./ComposerFooter";

type Props = {
  // Conversation data
  messages: Message[];
  plan: PlanItem[];
  toolCalls: Record<string, ToolCall>;
  selectedToolId: string;
  onSelectTool: (id: string) => void;

  // Composer
  composerValue: string;
  onComposerChange: (v: string) => void;
  onSend: (text: string) => void;
  attachments: Attachment[];
  onRemoveAttachment: (i: number) => void;
  mode: ComposerMode;
  onModeChange: (m: ComposerMode) => void;

  // Tab strip
  tabs: ChatTab[];
  viewTabs: ViewTab[];
  activeTabId: string;
  activeMainView: string | null;
  onSelectTab: (id: string) => void;
  onCloseTab: (id: string) => void;
  onSelectMainView: (id: string) => void;
  onCloseMainView: (id: string) => void;

  // Inline tool expansion
  expandedToolIds: Set<string>;
  onToggleExpand: (id: string) => void;
  onOpenInspector: (id: string) => void;
};

export function ChatPanel({
  messages, plan, toolCalls, selectedToolId, onSelectTool,
  composerValue, onComposerChange, onSend,
  attachments, onRemoveAttachment, mode, onModeChange,
  tabs, viewTabs, activeTabId, activeMainView,
  onSelectTab, onCloseTab, onSelectMainView, onCloseMainView,
  expandedToolIds, onToggleExpand, onOpenInspector,
}: Props) {
  const scrollRef = useRef<HTMLDivElement>(null);
  const inspectorTabs = useInspectorTabs();

  useEffect(() => {
    const el = scrollRef.current;
    if (!el) return;
    el.scrollTop = el.scrollHeight;
  }, [messages]);

  // When a workspace view tab is active, its body replaces the message
  // stream. Lookup chain: workspaceViews → inspectorTabs (auto-promoted).
  const activeViewBody = activeMainView
    ? (usePluginStore.getState().workspaceViews.get(activeMainView)?.value.component
       ?? inspectorTabs.find((t) => t.id === activeMainView)?.component
       ?? null)
    : null;

  const headerActiveId = activeMainView ?? activeTabId;

  return (
    <Panel className="chat">
      <ChatTopBar
        tabs={tabs}
        viewTabs={viewTabs}
        activeId={headerActiveId}
        onSelectChat={onSelectTab}
        onCloseChat={onCloseTab}
        onSelectView={onSelectMainView}
        onCloseView={onCloseMainView}
      />

      {activeViewBody ? (
        <PluginBoundary plugin={`workspace:${activeMainView}`} label="main view">
          {/* Promoted inspector tabs still call `useInspector()` —
              provide the same context here so they don't have to detect
              whether they're docked or in the main pane. */}
          <MainViewInspectorBridge>
            <ActiveView component={activeViewBody} />
          </MainViewInspectorBridge>
        </PluginBoundary>
      ) : (
        <MessageStream
          ref={scrollRef}
          messages={messages}
          ctx={{
            plan, toolCalls, selectedToolId, onSelectTool,
            expandedIds: expandedToolIds, onToggleExpand, onOpenInspector,
          }}
        />
      )}

      <div className="composer-wrap">
        <div className="composer-fade" />
        <div className="composer-inner">
          <Slot name="chat.status" />
          <SlashSuggestions value={composerValue} onPick={onComposerChange} />
          <Composer
            value={composerValue}
            onChange={onComposerChange}
            onSend={onSend}
            attachments={attachments}
            onRemoveAttachment={onRemoveAttachment}
            mode={mode}
            onModeChange={onModeChange}
          />
          <ComposerFooter />
        </div>
      </div>
    </Panel>
  );
}

// Trivial wrapper — needed because we resolved the component reference
// imperatively above and React likes proper component identity.
function ActiveView({ component: Body }: { component: React.ComponentType }) {
  return <Body />;
}

// Supplies the InspectorContext when an inspector tab is promoted to the
// main pane. Reads `activeFile` / setters from the same store the docked
// inspector uses, so switching files in either place stays in sync.
function MainViewInspectorBridge({ children }: { children: React.ReactNode }) {
  const activeFile = useUIStore((s) => s.activeFile);
  const setActiveFile = useUIStore((s) => s.setActiveFile);
  const setInspectorTab = useUIStore((s) => s.setInspectorTab);
  return (
    <InspectorProvider
      value={{ activeFile, onSelectFile: setActiveFile, onSwitchTab: setInspectorTab }}
    >
      {children}
    </InspectorProvider>
  );
}

export type { ComposerMode };
