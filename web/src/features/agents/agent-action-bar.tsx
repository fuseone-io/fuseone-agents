import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";
import { Eye, FlaskConical, Pencil, Play } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Separator } from "@/components/ui/separator";
import { AgentMoreMenu } from "@/features/agents/agent-more-menu";
import { AgentStateBlock } from "@/features/agents/agent-state-block";
import { AgentPrimary } from "@/features/agents/agent-primary";
import { RetireDialog } from "@/features/agents/retire-agent";
import { StageControl } from "@/features/agents/stage-control";
import type { Agent } from "@/lib/api/client";
import type { Stage } from "@/features/agents/stage-api";

/**
 * One row, one hierarchy at a time.
 *
 * Reading on the left, context in the middle, action on the right, and exactly
 * one filled button in the whole bar. What it replaces put six controls of
 * different natures at the same height with the same weight — Retire eight
 * pixels from Run, a two-line dropdown knocking the row out of alignment, and
 * a switch beside the word "Stopped" that read as neither state nor control.
 *
 * The primary keeps its place and changes its verb. Somebody learns one
 * position rather than four.
 */
/**
 * Written out rather than built from the value, so the guard that checks every
 * key exists can see them: a template literal is invisible to it.
 */
const MEANS: Record<Stage, string> = {
  draft: "stage.means.draft",
  copilot: "stage.means.copilot",
  autonomous: "stage.means.autonomous",
};

export function AgentActionBar({ agent }: { agent: Agent }) {
  const { t } = useTranslation();
  const [retiring, setRetiring] = useState(false);
  const retired = agent.retired ?? false;
  const running = !retired && agent.paused === false;

  return (
    <div className="flex h-14 items-center gap-3 rounded-xl border border-border bg-card px-4 shadow-sm">
      <AgentStateBlock agent={agent} />
      <Separator orientation="vertical" className="!h-6" />

      <StageControl agentId={agent.agentId} stage={agent.stage} />
      {/* The consequence of the stage, out of the control and beside it: it is
          worth saying and it was making the row two lines tall. */}
      <span className="flex items-center gap-1.5 text-2xs whitespace-nowrap text-muted-foreground">
        <Eye className="size-3.5" aria-hidden />
        {t(MEANS[(agent.stage ?? "draft") as Stage])}
      </span>

      <span className="flex-1" />

      <Button variant="ghost" size="sm" asChild className="h-9">
        <Link to={`/agents/${agent.agentId}/edit`}>
          <Pencil className="size-4" aria-hidden />
          {t("agents.edit")}
        </Link>
      </Button>
      <Button variant="outline" size="sm" asChild className="h-9">
        <Link to={`/agents/${agent.agentId}/simulate`}>
          <FlaskConical className="size-4" aria-hidden />
          {t("agents.simulate")}
        </Link>
      </Button>
      <AgentPrimary agent={agent} />

      <Separator orientation="vertical" className="!h-6" />
      <AgentMoreMenu
        running={running}
        agentId={agent.agentId}
        onRetire={() => setRetiring(true)}
        label={t("agents.moreActions")}
      />

      {retiring && (
        <RetireDialog
          agentId={agent.agentId}
          onClose={() => setRetiring(false)}
        />
      )}
    </div>
  );
}

/** An agent nobody may act on still reads, so the bar is not hidden. */
export function ReadOnlyBar({ agent }: { agent: Agent }) {
  const { t } = useTranslation();
  return (
    <div className="flex h-14 items-center gap-3 rounded-xl border border-border bg-card px-4 shadow-sm">
      <AgentStateBlock agent={agent} />
      <span className="flex-1" />
      <span className="flex items-center gap-1.5 text-2xs text-muted-foreground">
        <Play className="size-3.5" aria-hidden />
        {t("agents.readOnly")}
      </span>
    </div>
  );
}
