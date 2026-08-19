import { IdCard } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { ModelField, modelsFor } from "@/features/agents/model-field";
import { useIntegrations } from "@/features/integrations/api";
import { Section, Labelled } from "@/features/policies/section";
import { AgentAreaField } from "@/features/agents/agent-area-field";
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
  const integrations = useIntegrations().data;
  const providers = integrations?.providers ?? [];
  const presets = integrations?.presets ?? [];

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
          area={draft.area}
          onChange={(area) => patch({ area })}
        />
      </div>

      {/* Every track allowed to shrink below its content. A grid track is
          auto-sized to its minimum content by default, and a Select cannot
          shrink its own label — which is how one long option pushed this row
          out past the card it sits in. */}
      <div className="grid gap-3 sm:grid-cols-[minmax(0,200px)_minmax(0,1fr)_minmax(0,140px)]">
        {/* Offered from what this installation has actually configured. A
            typed provider name that nothing serves is an agent that publishes
            and fails on its first turn, naming a provider nobody connected. */}
        <Labelled
          label={t("agents.provider")}
          htmlFor="agent-provider"
          required={agentRequirementMarked("provider")}
        >
          <Select
            value={draft.provider}
            onValueChange={(provider) => patch({ provider, model: "" })}
          >
            <SelectTrigger
              id="agent-provider"
              className="font-mono"
              aria-required={agentRequirementMarked("provider")}
            >
              <SelectValue placeholder={t("agents.pickProvider")} />
            </SelectTrigger>
            <SelectContent>
              {providers.map((p) => (
                <SelectItem key={p.name} value={p.name} className="font-mono">
                  {p.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </Labelled>
        <Labelled
          label={t("agents.model")}
          htmlFor="agent-model"
          required={agentRequirementMarked("model")}
        >
          <ModelField
            id="agent-model"
            value={draft.model}
            options={modelsFor(draft.provider, providers, presets)}
            onChange={(model) => patch({ model })}
            required={agentRequirementMarked("model")}
          />
        </Labelled>
        <Labelled label={t("agents.effort")} htmlFor="agent-effort">
          <Select
            value={draft.effort || "none"}
            onValueChange={(effort) =>
              patch({ effort: effort === "none" ? "" : effort })
            }
          >
            <SelectTrigger id="agent-effort" className="min-w-0 font-mono">
              {/* Truncating rather than pushing: a select cannot shrink its
                  own text, and the one long option here was breaking the card
                  it sits in. */}
              <SelectValue className="truncate" />
            </SelectTrigger>
            <SelectContent>
              {["none", "low", "medium", "high"].map((level) => (
                <SelectItem key={level} value={level} className="font-mono">
                  {level === "none" ? t("agents.effortDefault") : level}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </Labelled>
      </div>
    </Section>
  );
}
