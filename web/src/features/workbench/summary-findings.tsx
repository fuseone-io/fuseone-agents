import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { useTools } from "@/features/admin/api";
import { agentTarget } from "@/features/agents/agent-target";
import { parse } from "@/features/agents/instruction-blocks";
import { FindingHeadline } from "@/features/agents/instruction-finding";
import { findings } from "@/features/agents/instruction-lint";
import { usePolicies } from "@/features/policies/api";
import type { AgentDefinition } from "@/lib/api/client";

/**
 * Where the instructions promise what will not happen: a tool they name that
 * a policy refuses, or one the agent does not hold. Findings, never refusals.
 * The instructions change only through the conversation, so a sentence is
 * the assistant's to rewrite; enabling a tool is a field, done here.
 */
export function SummaryFindings({
  local,
  patch,
}: {
  local: AgentDefinition;
  patch: (over: Partial<AgentDefinition>) => void;
}) {
  const { t } = useTranslation();
  const catalogue = useTools(local.company !== "", agentTarget(local));
  const policies = usePolicies();
  const tools = local.tools ?? [];
  const found = findings(parse(local.instructions), catalogue.data?.items ?? [], policies.data?.items ?? [], tools);
  if (found.length === 0) return null;

  return (
    <ul className="flex flex-col gap-2">
      {found.map((finding) => (
        <li key={`${finding.at}:${finding.tool}`} className="flex flex-col gap-1.5 rounded-lg border border-border p-3">
          <FindingHeadline finding={finding} />
          {finding.why === "notEnabled" ? (
            <Button size="sm" variant="outline" className="self-start" onClick={() => patch({ tools: [...tools, finding.tool] })}>
              {t("agents.enableTheTool")}
            </Button>
          ) : (
            <p className="text-2xs text-muted-foreground">{t("workbench.summary.refusedHint")}</p>
          )}
        </li>
      ))}
    </ul>
  );
}
