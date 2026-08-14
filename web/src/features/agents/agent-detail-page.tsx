import { useParams, useSearchParams } from "react-router-dom";
import { ErrorState, LoadingRows } from "@/components/shared/states";
import { useAgent } from "@/features/agents/agent-detail-api";
import { AgentIdentity } from "@/features/agents/agent-identity";
import {
  AgentActionBar,
  ReadOnlyBar,
} from "@/features/agents/agent-action-bar";
import { AgentKpis } from "@/features/agents/agent-kpis";
import { AgentDefinition } from "@/features/agents/agent-definition";
import { AgentCapabilities } from "@/features/agents/agent-capabilities";
import { AgentVersions } from "@/features/agents/agent-versions";
import { WebhooksPanel } from "@/features/agents/webhooks-panel";
import { AgentRuns } from "@/features/agents/agent-runs";

/**
 * One agent, as it was published.
 *
 * The definition is the centre of the screen because it is the thing that
 * explains every run underneath it: what somebody told the agent to do, in the
 * exact version those runs were pinned to. Everything else on the page is a
 * consequence of it.
 */
export function AgentDetailPage() {
  const { agentId = "" } = useParams();
  const [params] = useSearchParams();
  const version = params.get("version") ?? undefined;

  const agent = useAgent(agentId, version);

  if (agent.isLoading) return <LoadingRows rows={8} />;
  if (agent.error) {
    return (
      <ErrorState error={agent.error} onRetry={() => void agent.refetch()} />
    );
  }
  if (!agent.data) return null;

  const { agent: published, instructions, source, versions } = agent.data;

  return (
    <div className="flex w-full min-w-0 flex-col gap-5">
      <AgentIdentity agent={published} versions={versions} />
      {/* An older version is read, never operated: what a button here would
          act on is the newest, which is not the version being looked at. */}
      {published.latest ? (
        <AgentActionBar agent={published} />
      ) : (
        <ReadOnlyBar agent={published} />
      )}
      <AgentKpis agent={published} />

      <div className="grid gap-5 lg:grid-cols-[1fr_320px] lg:items-start">
        <AgentDefinition instructions={instructions} source={source} />
        <div className="flex flex-col gap-4">
          <AgentCapabilities agent={published} />
          <WebhooksPanel agentId={agentId} />
          <AgentVersions
            agentId={agentId}
            versions={versions}
            current={published.versionId}
          />
        </div>
      </div>

      <AgentRuns agentId={agentId} />
    </div>
  );
}
