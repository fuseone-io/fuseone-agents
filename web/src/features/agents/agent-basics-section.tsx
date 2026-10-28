import { IdCard } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Input } from "@/components/ui/input";
import { Section, Labelled } from "@/features/policies/section";
import { AgentAreaField } from "@/features/agents/agent-area-field";
import { AgentServingFields } from "@/features/agents/agent-serving-fields";
import { agentRequirementMarked } from "@/features/agents/agent-required";
import type { AgentDefinition } from "@/lib/api/client";

/** Who this agent is, and what it was told to do. */
export function AgentBasicsSection({
  draft,
  patch,
  agentId,
  editable,
  onAgentId,
}: {
  draft: AgentDefinition;
  patch: (over: Partial<AgentDefinition>) => void;
  agentId: string;
  editable: boolean;
  onAgentId: (id: string) => void;
}) {
  const { t } = useTranslation();

  return (
    <Section
      icon={IdCard}
      title={t("agents.identity")}
      hint={t("agents.areaIsUnit")}
    >
      <div className="grid gap-3 sm:grid-cols-[200px_1fr_160px]">
        <Labelled
          label={t("agents.identifier")}
          htmlFor="agent-id"
          required={agentRequirementMarked("identifier")}
        >
          {/* Set once: runs reference it forever, and an id that moved would
              orphan every one of them. */}
          <Input
            id="agent-id"
            value={agentId}
            onChange={(e) => onAgentId(e.target.value)}
            disabled={!editable}
            readOnly={!editable}
            className="font-mono"
            placeholder="suporte"
            required={agentRequirementMarked("identifier")}
          />
        </Labelled>
        <Labelled
          label={t("admin.name")}
          htmlFor="agent-name"
          required={agentRequirementMarked("name")}
        >
          <Input
            id="agent-name"
            value={draft.name}
            onChange={(e) => patch({ name: e.target.value })}
            placeholder={t("agents.namePlaceholder")}
            required={agentRequirementMarked("name")}
          />
        </Labelled>
        <AgentAreaField
          company={draft.company}
          area={draft.area}
          onChange={(scope) => patch(scope)}
        />
      </div>

      <AgentServingFields draft={draft} patch={patch} />
    </Section>
  );
}
