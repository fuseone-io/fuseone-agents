import { useTranslation } from "react-i18next";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Section, Labelled } from "@/features/policies/section";
import { AgentAreaField } from "@/features/agents/agent-area-field";
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
    <Section title="Identidade" hint={t("agents.areaIsUnit")}>
      <div className="grid gap-3 sm:grid-cols-[200px_1fr_160px]">
        <Labelled label="Identificador" htmlFor="agent-id">
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
          />
        </Labelled>
        <Labelled label={t("admin.name")} htmlFor="agent-name">
          <Input
            id="agent-name"
            value={draft.name}
            onChange={(e) => patch({ name: e.target.value })}
            placeholder="Atendimento de suporte"
          />
        </Labelled>
        <AgentAreaField
          area={draft.area}
          onChange={(area) => patch({ area })}
        />
      </div>

      <div className="grid gap-3 sm:grid-cols-[1fr_1fr_140px]">
        <Labelled label={t("agents.provider")} htmlFor="agent-provider">
          <Input
            id="agent-provider"
            value={draft.provider}
            onChange={(e) => patch({ provider: e.target.value })}
            className="font-mono"
            placeholder="openai"
          />
        </Labelled>
        <Labelled label="Modelo" htmlFor="agent-model">
          <Input
            id="agent-model"
            value={draft.model}
            onChange={(e) => patch({ model: e.target.value })}
            className="font-mono"
          />
        </Labelled>
        <Labelled label={t("agents.effort")} htmlFor="agent-effort">
          <Input
            id="agent-effort"
            value={draft.effort ?? ""}
            onChange={(e) => patch({ effort: e.target.value })}
            className="font-mono"
            placeholder="low"
          />
        </Labelled>
      </div>

      <Labelled label={t("agents.instructions")} htmlFor="agent-instructions">
        <Textarea
          id="agent-instructions"
          rows={8}
          value={draft.instructions}
          onChange={(e) => patch({ instructions: e.target.value })}
          className="font-mono text-xs"
          placeholder={t("agents.instructionsPlaceholder")}
        />
        <p className="text-xs text-muted-foreground">
          {t("agents.instructionsLength", {
            count: draft.instructions.trim().length,
          })}
        </p>
      </Labelled>
    </Section>
  );
}
