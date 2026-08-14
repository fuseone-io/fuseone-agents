import { useTranslation } from "react-i18next";
import { ListOrdered } from "lucide-react";
import { Button } from "@/components/ui/button";
import { AgentBasicsSection } from "@/features/agents/agent-basics-section";
import { InstructionsEditor } from "@/features/agents/instructions-editor";
import type { AgentDefinition } from "@/lib/api/client";

/**
 * Who the agent is, and what it is told to do.
 *
 * The instructions get the full width and no second column, because they are
 * the artefact an auditor reads to understand a run — a preview pane beside
 * them would halve the thing that matters to show a rendering of itself.
 */
export function TabDefinition({
  draft,
  patch,
  editing,
  onSteps,
}: {
  draft: AgentDefinition;
  patch: (over: Partial<AgentDefinition>) => void;
  editing: {
    agentId: string;
    creating: boolean;
    onAgentId: (id: string) => void;
  };
  /** Opens the tab where this text is read as stages. */
  onSteps: () => void;
}) {
  return (
    <>
      <AgentBasicsSection
        draft={draft}
        patch={patch}
        agentId={editing.agentId}
        editable={editing.creating}
        onAgentId={editing.onAgentId}
      />

      <InstructionsEditor
        instructions={draft.instructions}
        onChange={(instructions) => patch({ instructions })}
      />

      <ReadAs steps={(draft.steps ?? []).length} onOpen={onSteps} />
    </>
  );
}

/**
 * How many stages this text was read as, and the way to go and look.
 *
 * The connection between the two halves, stated once and in the direction
 * that is true: the stages came out of this text. Numbering the lines would
 * claim the reverse — that the text is already a list — and it is not.
 */
function ReadAs({ steps, onOpen }: { steps: number; onOpen: () => void }) {
  const { t } = useTranslation();

  return (
    <div className="flex items-center gap-2 text-2xs text-muted-foreground">
      <ListOrdered className="size-3.5 shrink-0" aria-hidden />
      <span>
        {steps === 0
          ? t("agents.notReadYet")
          : t("agents.readAsSteps", { count: steps })}
      </span>
      <Button
        type="button"
        variant="link"
        size="sm"
        className="h-auto p-0 text-2xs"
        onClick={onOpen}
      >
        {t("agents.openSteps")}
      </Button>
    </div>
  );
}
