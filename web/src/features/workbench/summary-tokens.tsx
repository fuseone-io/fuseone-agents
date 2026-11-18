import { useTranslation } from "react-i18next";
import { Mono } from "@/components/shared/mono";
import { agentTarget } from "@/features/agents/agent-target";
import { useInstructionTokens } from "@/features/agents/instruction-tokens-api";
import type { AgentDefinition } from "@/lib/api/client";

/**
 * How large the instructions are, counted by the agent's own provider and
 * model. Nothing while nothing could count them: characters printed as
 * tokens would be a number that means something else.
 */
export function SummaryTokens({ local }: { local: AgentDefinition }) {
  const { t } = useTranslation();
  const tokens = useInstructionTokens(local.provider, local.model, local.instructions, agentTarget(local));
  if (tokens === undefined) return null;
  return <Mono dim className="text-xs">{t("agents.tokensCounted", { count: tokens })}</Mono>;
}
