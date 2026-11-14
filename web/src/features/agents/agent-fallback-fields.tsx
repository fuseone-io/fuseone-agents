import { useTranslation } from "react-i18next";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { ModelField, modelsFor } from "@/features/agents/model-field";
import { agentTarget } from "@/features/agents/agent-target";
import { ProviderOptions } from "@/features/agents/provider-options";
import { useProviderChoices } from "@/features/agents/providers-api";
import { Labelled } from "@/features/policies/section";
import type { AgentDefinition } from "@/lib/api/client";

const NONE = "none";

/**
 * The pair a draft or copilot run switches to when the declared one keeps
 * failing as overloaded, rate limited or unavailable.
 *
 * Offered from the providers the agent's scope reaches, beside the
 * declared pair, because publishing refuses any other: the moment an unknown
 * fallback would otherwise be found out is the outage it exists for.
 */
export function AgentFallbackFields({
  draft,
  patch,
}: {
  draft: AgentDefinition;
  patch: (over: Partial<AgentDefinition>) => void;
}) {
  const { t } = useTranslation();
  // The same answer the serving fields ask for, which also say when it fails.
  const providers = useProviderChoices(agentTarget(draft)).data?.items ?? [];
  const fallback = draft.fallback;

  return (
    <div className="grid gap-3 sm:grid-cols-[minmax(0,200px)_minmax(0,1fr)]">
      <Labelled label={t("agents.fallbackProvider")} htmlFor="agent-fallback-provider">
        <Select
          value={fallback?.provider || NONE}
          onValueChange={(provider) =>
            patch({
              fallback: provider === NONE ? undefined : { provider, model: "" },
            })
          }
        >
          <SelectTrigger id="agent-fallback-provider" className="w-full min-w-0 font-mono">
            <SelectValue className="truncate" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={NONE}>{t("agents.fallbackNone")}</SelectItem>
            <ProviderOptions providers={providers} current={fallback?.provider} />
          </SelectContent>
        </Select>
      </Labelled>
      {fallback && (
        <Labelled label={t("agents.fallbackModel")} htmlFor="agent-fallback-model">
          <ModelField
            id="agent-fallback-model"
            value={fallback.model}
            options={modelsFor(fallback.provider, providers, [])}
            onChange={(model) => patch({ fallback: { ...fallback, model } })}
          />
        </Labelled>
      )}
      {fallback && (
        <p className="text-sm text-muted-foreground sm:col-span-2">
          {t("agents.fallbackHint")}
        </p>
      )}
    </div>
  );
}
