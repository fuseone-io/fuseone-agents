import { useTranslation } from "react-i18next";
import { FileText, ListOrdered } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { Section } from "@/features/policies/section";
import { AgentBasicsSection } from "@/features/agents/agent-basics-section";
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
  const { t } = useTranslation();

  return (
    <>
      <AgentBasicsSection
        draft={draft}
        patch={patch}
        agentId={editing.agentId}
        editable={editing.creating}
        onAgentId={editing.onAgentId}
      />

      <Section
        icon={FileText}
        title={t("agents.instructions")}
        hint={t("agents.instructionsHint")}
        action={
          <span className="font-mono text-2xs tabular-nums text-muted-foreground">
            {t("agents.instructionsLength", {
              count: draft.instructions.trim().length,
            })}
          </span>
        }
      >
        {/* An editor surface, and deliberately without line numbers. A number
            in the gutter says a line is addressable, which teaches that one
            line is one step — and the moment somebody writes two in a line, or
            wraps one across three, the numbering is a claim about structure
            that the steps disagree with. The prose is what the model reads;
            what the Gate obeys is next door and says so below. */}
        <Textarea
          id="agent-instructions"
          rows={12}
          value={draft.instructions}
          onChange={(e) => patch({ instructions: e.target.value })}
          className="resize-y bg-muted/40 font-mono text-xs leading-relaxed"
          placeholder={t("agents.instructionsPlaceholder")}
          aria-label={t("agents.instructions")}
          spellCheck={false}
        />

        <ReadAs steps={(draft.steps ?? []).length} onOpen={onSteps} />
      </Section>
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
