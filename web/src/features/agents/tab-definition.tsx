import { useTranslation } from "react-i18next";
import { FileText, IdCard } from "lucide-react";
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
  agentId,
  creating,
  onAgentId,
}: {
  draft: AgentDefinition;
  patch: (over: Partial<AgentDefinition>) => void;
  agentId: string;
  creating: boolean;
  onAgentId: (id: string) => void;
}) {
  const { t } = useTranslation();

  return (
    <>
      <AgentBasicsSection
        draft={draft}
        patch={patch}
        agentId={agentId}
        editable={creating}
        onAgentId={onAgentId}
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
        <Textarea
          id="agent-instructions"
          rows={8}
          value={draft.instructions}
          onChange={(e) => patch({ instructions: e.target.value })}
          className="font-mono text-xs"
          placeholder={t("agents.instructionsPlaceholder")}
          aria-label={t("agents.instructions")}
        />
      </Section>
    </>
  );
}

/** Kept so the icon the tab bar names is the icon the card carries. */
export const DEFINITION_ICON = IdCard;
