import { Trans, useTranslation } from "react-i18next";
import { FlaskConical, Pencil } from "lucide-react";
import { Link } from "react-router-dom";
import { Button } from "@/components/ui/button";
import { Separator } from "@/components/ui/separator";
import { Mono } from "@/components/shared/mono";
import { StateDot } from "@/components/shared/state-dot";
import { stateOfAgent } from "@/lib/agent-state";
import { RunNowDialog } from "@/features/agents/run-now-dialog";
import { StageControl } from "@/features/agents/stage-control";
import { formatInstant } from "@/lib/format";
import type { Agent, AgentVersion, Phase } from "@/lib/api/client";

/**
 * Which agent this is, and which version of it you are reading.
 *
 * The version is stated rather than implied: a reader who arrived from a run
 * is looking at the text that run executed, which may not be the newest.
 */
export function AgentIdentity({
  agent,
  versions,
}: {
  agent: Agent;
  versions: AgentVersion[];
}) {
  const { t } = useTranslation();
  const superseded = !agent.latest && versions.length > 1;

  return (
    <div className="flex flex-wrap items-start gap-4">
      <div className="flex min-w-0 flex-col gap-1.5">
        <div className="flex items-center gap-2.5">
          <h1 className="text-2xl font-medium tracking-display">
            {agent.name}
          </h1>
          <span className="inline-flex h-6 items-center gap-1.5 rounded-pill bg-muted px-2.5 text-xs font-medium">
            <StateDot
              state={stateOfAgent(
                agent.activity?.lastPhase as Phase | undefined,
              )}
            />
            {agent.scope.area || t("agents.noArea")}
          </span>
          {superseded && (
            <span className="inline-flex h-6 items-center rounded-pill bg-warning-surface px-2.5 text-xs font-medium text-warning">
              {t("agents.oldVersion")}
            </span>
          )}
        </div>

        <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
          <Mono dim>{agent.agentId}</Mono>
          <Separator orientation="vertical" className="!h-3" />
          <span>
            <Trans
              i18nKey="agents.publishedLine"
              values={{
                version: agent.versionId.slice(0, 9),
                model: `${agent.provider}/${agent.model}`,
                by: agent.publishedBy ?? "",
                at: formatInstant(agent.publishedAt),
              }}
              components={{ v: <Mono dim /> }}
            />
          </span>
        </div>
      </div>

      <div className="ml-auto flex shrink-0 items-center gap-2">
        {/* An older version is read, never re-run: what would open is the
            newest, which is not the version being looked at. */}
        {agent.latest ? (
          <>
            <Button variant="outline" size="sm" asChild className="h-8">
              <Link to={`/agents/${agent.agentId}/edit`}>
                <Pencil className="size-4" aria-hidden />
                {t("agents.edit")}
              </Link>
            </Button>
            {/* Beside running it, not buried: simulating is how an agent
                earns its way out of Draft, and running it for real is the
                thing somebody does instead when they cannot find this. */}
            <Button variant="outline" size="sm" asChild className="h-8">
              <Link to={`/agents/${agent.agentId}/simulate`}>
                <FlaskConical className="size-4" aria-hidden />
                {t("agents.simulate")}
              </Link>
            </Button>
            <StageControl agentId={agent.agentId} stage={agent.stage} />
            <RunNowDialog agentId={agent.agentId} agentName={agent.name} />
          </>
        ) : (
          <Button variant="outline" size="sm" disabled className="h-8">
            {t("agents.readOnly")}
          </Button>
        )}
      </div>
    </div>
  );
}
