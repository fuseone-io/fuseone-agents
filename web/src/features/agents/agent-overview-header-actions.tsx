import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";
import { FlaskConical, Pencil, Play } from "lucide-react";
import { Button } from "@/components/ui/button";
import { AgentMoreMenu } from "@/features/agents/agent-more-menu";
import { AgentPrimary } from "@/features/agents/agent-primary";
import { RetireDialog } from "@/features/agents/retire-agent";
import type { Agent } from "@/lib/api/client";

export function HeaderActions({ agent }: { agent: Agent }) {
  const { t } = useTranslation();
  const [retiring, setRetiring] = useState(false);
  const retired = agent.retired ?? false;
  const running = !retired && agent.paused === false;

  if (!agent.latest) {
    return (
      <span className="inline-flex h-9 items-center gap-1.5 text-2xs text-muted-foreground">
        <Play className="size-3.5" aria-hidden />
        {t("agents.readOnly")}
      </span>
    );
  }

  return (
    <div className="flex shrink-0 flex-wrap items-center justify-end gap-2">
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
