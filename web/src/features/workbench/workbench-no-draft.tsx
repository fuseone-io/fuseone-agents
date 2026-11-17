import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";
import { toast } from "sonner";
import { FilePlus2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { EmptyState, ErrorState, LoadingRows } from "@/components/shared/states";
import { useAgent } from "@/features/agents/agent-detail-api";
import { isAbsent, useCreateDraft } from "@/features/workbench/api";
import { problemMessage } from "@/lib/api/problem-message";

/**
 * An agent with no draft: a published one opens a draft from its current
 * version, untouched; an identifier with nothing behind it is pointed to the
 * first step of a new agent.
 */
export function WorkbenchNoDraft({ agentId }: { agentId: string }) {
  const { t } = useTranslation();
  const agent = useAgent(agentId, undefined);
  const create = useCreateDraft();

  if (agent.isLoading) return <LoadingRows rows={3} />;
  if (agent.error && !isAbsent(agent.error)) {
    return <ErrorState error={agent.error} onRetry={() => void agent.refetch()} />;
  }
  if (!agent.data) {
    return (
      <div className="flex flex-col items-center gap-4">
        <EmptyState title={t("workbench.noAgentTitle")} hint={t("workbench.noAgentHint")} />
        <Button asChild variant="outline">
          <Link to="/agents/workbench">{t("workbench.newAgent")}</Link>
        </Button>
      </div>
    );
  }
  const open = () =>
    create.mutate({ agentId }, { onError: (error) => toast.error(problemMessage(error, t)) });

  return (
    <div className="flex flex-col items-center gap-4">
      <EmptyState title={t("workbench.noDraftTitle")} hint={t("workbench.noDraftHint")} />
      <Button onClick={open} disabled={create.isPending}>
        <FilePlus2 className="size-4" />
        {t("workbench.openDraft")}
      </Button>
    </div>
  );
}
