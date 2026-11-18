import { useTranslation } from "react-i18next";
import { Mono } from "@/components/shared/mono";
import { agentTarget } from "@/features/agents/agent-target";
import { useInstructionTokens } from "@/features/agents/instruction-tokens-api";
import type { AgentDefinition } from "@/lib/api/client";

/**
 * How large the instructions are, in whichever unit was measured. Tokens
 * belong to the model that will read them, so they are the agent's own
 * provider's count; where that provider counts nothing, this says characters
 * and means characters. Characters printed as tokens would be a wrong number
 * in the one place somebody goes to size a prompt.
 */
export function SummaryTokens({ local }: { local: AgentDefinition }) {
  const { t } = useTranslation();
  const tokens = useInstructionTokens(local.provider, local.model, local.instructions, agentTarget(local));
  const size =
    tokens !== undefined
      ? t("agents.tokensCounted", { count: tokens })
      : t("agents.charactersCounted", { count: local.instructions.trim().length });
  return <Mono dim className="text-xs">{size}</Mono>;
}
