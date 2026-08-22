import { useParams, useSearchParams } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { ErrorState, LoadingRows } from "@/components/shared/states";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { useAgent } from "@/features/agents/agent-detail-api";
import { AgentOverviewHeader } from "@/features/agents/agent-overview-header";
import { AgentDefinition } from "@/features/agents/agent-definition";
import { AgentCapabilities } from "@/features/agents/agent-capabilities";
import { AgentVersions } from "@/features/agents/agent-versions";
import { WebhooksPanel } from "@/features/agents/webhooks-panel";
import { AgentRuns } from "@/features/agents/agent-runs";
import { AgentGuidedPath } from "@/features/agents/agent-guided-path";
import { publishedAgentGuideSteps } from "@/features/agents/agent-guided-path-model";
import { useTools } from "@/features/admin/api";
import { useChannels } from "@/features/channels/api";
import { useMCPUserCredentials } from "@/features/integrations/api";
import { useRecipes } from "@/features/integrations/mcp/api";

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
  const publishedPreview = agent.data?.agent;
  const hasTools = (publishedPreview?.tools ?? []).length > 0;
  const hasChannelTrigger = (publishedPreview?.triggers ?? []).some(
    (trigger) => trigger.type === "channel",
  );
  const tools = useTools();
  const recipes = useRecipes(hasTools);
  const credentials = useMCPUserCredentials(hasTools);
  const channels = useChannels(hasChannelTrigger);

  if (agent.isLoading) return <LoadingRows rows={8} />;
  if (agent.error) {
    return (
      <ErrorState error={agent.error} onRetry={() => void agent.refetch()} />
    );
  }
  if (!agent.data) return null;

  const { agent: published, instructions, source, steps, versions } = agent.data;
  const guide = publishedAgentGuideSteps(published, instructions, {
    agentId,
    catalogue: tools.data?.items,
    recipes: recipes.data?.items,
    credentials: credentials.data?.items,
    channels: channels.data?.items,
    simulationTo: `/agents/${agentId}/simulate`,
  });

  return (
    <Tabs
      defaultValue="runs"
      className="mx-auto flex w-full max-w-[1500px] min-w-0 flex-col gap-4"
    >
      <AgentOverviewHeader
        agent={published}
        versions={versions}
        tabs={
          <TabsList
            variant="line"
            className="h-10 w-full justify-start overflow-x-auto"
            aria-label={t("agents.agentSections")}
          >
            <TabsTrigger value="runs" className="flex-none px-3">
              {t("agents.runs")}
              <span className="rounded-full bg-muted px-1.5 text-2xs text-muted-foreground">
                {published.activity?.runs ?? 0}
              </span>
            </TabsTrigger>
            <TabsTrigger value="definition" className="flex-none px-3">
              {t("agents.definition")}
            </TabsTrigger>
            <TabsTrigger value="steps" className="flex-none px-3">
              {t("agents.asSteps_other", { count: steps?.length ?? 0 })}
            </TabsTrigger>
          </TabsList>
        }
      />
      <AgentGuidedPath
        steps={guide}
        titleKey="agents.launchGuideTitle"
        subtitleKey="agents.launchGuideSubtitle"
        progressKey="agents.launchGuideProgress"
      />

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
  );
}
