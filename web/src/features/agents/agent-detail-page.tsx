import { useParams, useSearchParams } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { ErrorState, LoadingRows } from "@/components/shared/states";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { useAgent } from "@/features/agents/agent-detail-api";
import { AgentIdentity } from "@/features/agents/agent-identity";
import {
  AgentActionBar,
  ReadOnlyBar,
} from "@/features/agents/agent-action-bar";
import { AgentOverviewSummary } from "@/features/agents/agent-overview-summary";
import { AgentDefinition } from "@/features/agents/agent-definition";
import { AgentCapabilities } from "@/features/agents/agent-capabilities";
import { AgentVersions } from "@/features/agents/agent-versions";
import { WebhooksPanel } from "@/features/agents/webhooks-panel";
import { AgentRuns } from "@/features/agents/agent-runs";

/**
 * One agent, as it was published.
 *
 * Runs are the first answer.
 *
 * The definition explains a run, but it changes rarely. Somebody opening this
 * page usually needs to know whether the current agent is behaving, so the run
 * list is the default and the definition is still one tab away.
 */
export function AgentDetailPage() {
  const { agentId = "" } = useParams();
  const [params] = useSearchParams();
  const { t } = useTranslation();
  const version = params.get("version") ?? undefined;

  const agent = useAgent(agentId, version);

  if (agent.isLoading) return <LoadingRows rows={8} />;
  if (agent.error) {
    return (
      <ErrorState error={agent.error} onRetry={() => void agent.refetch()} />
    );
  }
  if (!agent.data) return null;

  const { agent: published, instructions, source, steps, versions } = agent.data;

  return (
    <div className="mx-auto flex w-full max-w-[1500px] min-w-0 flex-col gap-4">
      <AgentIdentity agent={published} versions={versions} />
      {/* An older version is read, never operated: what a button here would
          act on is the newest, which is not the version being looked at. */}
      {published.latest ? (
        <AgentActionBar agent={published} />
      ) : (
        <ReadOnlyBar agent={published} />
      )}
      <AgentOverviewSummary agent={published} />

      <Tabs defaultValue="runs" className="min-w-0 gap-4">
        <TabsList variant="line" className="h-9 w-full justify-start">
          <TabsTrigger value="runs">
            {t("agents.runs")}
            <span className="rounded-full bg-muted px-1.5 text-2xs text-muted-foreground">
              {published.activity?.runs ?? 0}
            </span>
          </TabsTrigger>
          <TabsTrigger value="definition">{t("agents.definition")}</TabsTrigger>
          <TabsTrigger value="steps">
            {t("agents.asSteps_other", { count: steps?.length ?? 0 })}
          </TabsTrigger>
        </TabsList>

        <div className="grid min-w-0 gap-4 xl:grid-cols-[minmax(0,1fr)_300px] xl:items-start">
          <div className="min-w-0">
            <TabsContent value="runs" className="mt-0">
              <AgentRuns agentId={agentId} showHeader={false} />
            </TabsContent>
            <TabsContent value="definition" className="mt-0">
              <AgentDefinition
                instructions={instructions}
                source={source}
                steps={steps}
                view="instructions"
              />
            </TabsContent>
            <TabsContent value="steps" className="mt-0">
              <AgentDefinition
                instructions={instructions}
                source={source}
                steps={steps}
                view="steps"
              />
            </TabsContent>
          </div>

          <aside className="flex min-w-0 flex-col gap-4">
            <AgentCapabilities agent={published} compact />
            <WebhooksPanel agentId={agentId} />
            <AgentVersions
              agentId={agentId}
              versions={versions}
              current={published.versionId}
              compact
            />
          </aside>
        </div>
      </Tabs>
    </div>
  );
}
