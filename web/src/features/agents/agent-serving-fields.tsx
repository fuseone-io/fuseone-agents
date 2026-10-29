import { useTranslation } from "react-i18next";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { ModelField, modelsFor } from "@/features/agents/model-field";
import { useIntegrations } from "@/features/integrations/api";
import { Labelled } from "@/features/policies/section";
import { agentRequirementMarked } from "@/features/agents/agent-required";
import type { AgentDefinition } from "@/lib/api/client";

/** How the agent is served: provider, model, and the knobs on that call. */
export function AgentServingFields({
  draft,
  patch,
}: {
  draft: AgentDefinition;
  patch: (over: Partial<AgentDefinition>) => void;
}) {
  const { t } = useTranslation();
  const integrations = useIntegrations().data;
  const providers = integrations?.providers ?? [];
  const presets = integrations?.presets ?? [];

  return (
    /* Every track allowed to shrink below its content. A grid track is
       auto-sized to its minimum content by default, and a Select cannot
       shrink its own label — which is how one long option pushed this row
       out past the card it sits in. */
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
            className="w-full font-mono"
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
          {/* w-full is the other half of min-w-0: the primitive's trigger is
              w-fit, so a long selected label grows it past its grid track and
              over the neighbouring select, and the primitive's line-clamp
              only truncates a trigger whose width is bounded (#167). */}
          <SelectTrigger id="agent-effort" className="w-full min-w-0 font-mono">
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
      <Labelled label={t("agents.cacheTtl")} htmlFor="agent-cache-ttl">
        <Select
          value={draft.cachePrefixTtl || "none"}
          onValueChange={(ttl) =>
            patch({ cachePrefixTtl: ttl === "none" ? "" : ttl })
          }
        >
          <SelectTrigger id="agent-cache-ttl" className="w-full min-w-0 font-mono">
            <SelectValue className="truncate" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="none" className="font-mono">
              {t("agents.cacheTtlDefault")}
            </SelectItem>
            <SelectItem value="1h" className="font-mono">
              {t("agents.cacheTtlHour")}
            </SelectItem>
          </SelectContent>
        </Select>
      </Labelled>
      <Labelled label={t("agents.replyLocale")} htmlFor="agent-reply-locale">
        <Select
          value={draft.replyLocale || "none"}
          onValueChange={(locale) =>
            patch({ replyLocale: locale === "none" ? "" : locale })
          }
        >
          <SelectTrigger id="agent-reply-locale" className="w-full min-w-0 font-mono">
            <SelectValue className="truncate" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="none" className="font-mono">
              {t("agents.replyLocaleDefault")}
            </SelectItem>
            {["pt-BR", "en-US"].map((locale) => (
              <SelectItem key={locale} value={locale} className="font-mono">
                {locale}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </Labelled>
    </div>
  );
}
