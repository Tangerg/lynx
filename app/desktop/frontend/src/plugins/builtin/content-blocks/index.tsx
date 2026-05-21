// Built-in plugins: renderers for the six standard content-block kinds.
//
// One plugin per kind so a user can replace any individual renderer
// (e.g. their own diff-aware code block) without taking out the others.
// Co-located here because each one is ~5 lines — a folder per plugin was
// pure overhead.

import { ApprovalCard } from "@/components/chat/ApprovalCard";
import { Checkpoint } from "@/components/chat/Checkpoint";
import { ShikiCodeBlock } from "@/components/chat/ShikiCodeBlock";
import { PlanBlock } from "@/components/chat/PlanBlock";
import { ReasoningBlock } from "@/components/chat/ReasoningBlock";
import { SearchResults } from "@/components/chat/SearchResults";
import { definePlugin, type ContentBlockRendererProps } from "@/plugins/sdk";
import { useAgentStore } from "@/state/agentStore";

export const approvalBlock = definePlugin({
  name: "lyra.builtin.approval-block",
  version: "1.0.0",
  setup({ host }) {
    host.message.registerContentBlock("approval", ({ block }: ContentBlockRendererProps<"approval">) => (
      <ApprovalCard what={block.text} cmd={block.command} reason={block.reason} />
    ));
  },
});

export const checkpointBlock = definePlugin({
  name: "lyra.builtin.checkpoint-block",
  version: "1.0.0",
  setup({ host }) {
    host.message.registerContentBlock("checkpoint", ({ block }: ContentBlockRendererProps<"checkpoint">) => (
      <Checkpoint text={block.text} />
    ));
  },
});

export const codeBlock = definePlugin({
  name: "lyra.builtin.code-block",
  version: "1.0.0",
  setup({ host }) {
    host.message.registerContentBlock("code", ({ block }: ContentBlockRendererProps<"code">) => (
      <ShikiCodeBlock lang={block.lang} code={block.text} file={block.file} />
    ));
  },
});

// The plan block carries no data of its own — it just marks "render the
// current plan here", and the renderer pulls from useAgentStore so plan
// updates re-render the block in place.
function PlanContentBlock() {
  const plan = useAgentStore((s) => s.plan);
  return <PlanBlock plan={plan} />;
}

export const planBlock = definePlugin({
  name: "lyra.builtin.plan-block",
  version: "1.0.0",
  setup({ host }) {
    host.message.registerContentBlock("plan", PlanContentBlock);
  },
});

export const reasoningBlock = definePlugin({
  name: "lyra.builtin.reasoning-block",
  version: "1.0.0",
  setup({ host }) {
    host.message.registerContentBlock("reasoning", ({ block }: ContentBlockRendererProps<"reasoning">) => (
      <ReasoningBlock text={block.text} streaming={block.streaming} />
    ));
  },
});

export const searchBlock = definePlugin({
  name: "lyra.builtin.search-block",
  version: "1.0.0",
  setup({ host }) {
    host.message.registerContentBlock("search", ({ block }: ContentBlockRendererProps<"search">) => (
      <SearchResults results={block.results} />
    ));
  },
});
