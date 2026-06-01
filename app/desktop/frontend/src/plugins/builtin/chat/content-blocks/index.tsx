// Built-in plugins: renderers for the six standard content-block kinds.
//
// One plugin per kind so a user can replace any individual renderer
// (e.g. their own diff-aware code block) without taking out the others.
// Co-located here because each one is ~5 lines — a folder per plugin was
// pure overhead.

import type { ContentBlockRendererProps } from "@/plugins/sdk";
import { ApprovalCard } from "@/components/chat/ApprovalCard";
import { Checkpoint } from "@/components/chat/Checkpoint";
import { PlanBlock } from "@/components/chat/PlanBlock";
import { QuestionCard } from "@/components/chat/QuestionCard";
import { ReasoningBlock } from "@/components/chat/ReasoningBlock";
import { SearchResults } from "@/components/chat/SearchResults";
import { ShikiCodeBlock } from "@/components/chat/ShikiCodeBlock";
import { definePlugin } from "@/plugins/sdk";
import { useAgentSlice } from "@/state/agentStore";

export const approvalBlock = definePlugin({
  name: "lyra.builtin.approval-block",
  version: "1.0.0",
  setup({ host }) {
    host.message.registerContentBlock(
      "approval",
      ({ block }: ContentBlockRendererProps<"approval">) => (
        <ApprovalCard
          status={block.status}
          what={block.text}
          cmd={block.command}
          reason={block.reason}
          requestId={block.requestId}
          decision={block.decision}
          args={block.args}
          risk={block.risk}
          scope={block.scope}
          target={block.target}
          reversible={block.reversible}
        />
      ),
    );
  },
});

export const questionBlock = definePlugin({
  name: "lyra.builtin.question-block",
  version: "1.0.0",
  setup({ host }) {
    host.message.registerContentBlock(
      "question",
      ({ block }: ContentBlockRendererProps<"question">) => (
        <QuestionCard
          status={block.status}
          requestId={block.requestId}
          questions={block.questions}
          answered={block.answered}
        />
      ),
    );
  },
});

export const checkpointBlock = definePlugin({
  name: "lyra.builtin.checkpoint-block",
  version: "1.0.0",
  setup({ host }) {
    host.message.registerContentBlock(
      "checkpoint",
      ({ block }: ContentBlockRendererProps<"checkpoint">) => <Checkpoint text={block.text} />,
    );
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
  const plan = useAgentSlice((v) => v.plan);
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
    host.message.registerContentBlock(
      "reasoning",
      ({ block }: ContentBlockRendererProps<"reasoning">) => (
        <ReasoningBlock text={block.text} status={block.status} />
      ),
    );
  },
});

export const searchBlock = definePlugin({
  name: "lyra.builtin.search-block",
  version: "1.0.0",
  setup({ host }) {
    host.message.registerContentBlock(
      "search",
      ({ block }: ContentBlockRendererProps<"search">) => <SearchResults results={block.results} />,
    );
  },
});
