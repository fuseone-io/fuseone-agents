import { Check, Copy } from "lucide-react";
import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Separator } from "@/components/ui/separator";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { Mono } from "@/components/shared/mono";
import { StateDot } from "@/components/shared/state-dot";
import { RunNowDialog } from "@/features/agents/run-now-dialog";
import { VerifyButton } from "@/features/runs/verify-button";
import { PHASE_LABELS } from "@/features/runs/phase-badge";
import { stateOfPhase } from "@/lib/agent-state";
import { formatInstant } from "@/lib/format";
import type { Run } from "@/lib/api/client";

/**
 * Which run this is, in one row.
 *
 * The identifier is copyable because it is what somebody pastes into a ticket,
 * a support thread or a query — and reading a thirty-character id off a screen
 * by hand is how the wrong run ends up in the incident report.
 */
export function RunIdentity({ run, trigger }: { run: Run; trigger?: string }) {
  return (
    <div className="flex flex-wrap items-start gap-4">
      <div className="min-w-0 flex flex-col gap-1.5">
        <div className="flex items-center gap-2.5">
          <h1 className="text-2xl font-medium tracking-display">{run.agentId}</h1>
          <span className="inline-flex h-6 items-center gap-1.5 rounded-pill bg-muted px-2.5 text-xs font-medium">
            <StateDot state={stateOfPhase(run.phase)} />
            {PHASE_LABELS[run.phase]}
          </span>
        </div>

        <div className="flex flex-wrap items-center gap-2">
          <Mono dim>{run.runId}</Mono>
          <CopyId value={run.runId} />
          <Separator orientation="vertical" className="!h-3" />
          <span className="text-xs text-muted-foreground">
            versão <Mono dim>{run.versionId.slice(0, 9)}</Mono>
            {trigger ? ` · gatilho ${trigger}` : ""} · iniciada {formatInstant(run.startedAt)}
            {run.onBehalfOf ? ` · em nome de ${run.onBehalfOf}` : ""}
          </span>
        </div>
      </div>

      <div className="ml-auto flex shrink-0 items-center gap-2">
        {/* A new run of the same agent, on the version published now — not a
            replay. Replaying would re-execute effects that already happened
            against systems that already changed. */}
        <RunNowDialog agentId={run.agentId} agentName={run.agentId} />
        <VerifyButton runId={run.runId} />
      </div>
    </div>
  );
}

function CopyId({ value }: { value: string }) {
  const [copied, setCopied] = useState(false);

  const copy = async () => {
    await navigator.clipboard.writeText(value);
    setCopied(true);
    window.setTimeout(() => setCopied(false), 1600);
  };

  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          variant="ghost"
          size="icon"
          className="size-6 text-muted-foreground"
          onClick={() => void copy()}
        >
          {copied ? <Check className="size-3.5" /> : <Copy className="size-3.5" />}
          <span className="sr-only">Copiar identificador da execução</span>
        </Button>
      </TooltipTrigger>
      <TooltipContent>{copied ? "Copiado" : "Copiar identificador"}</TooltipContent>
    </Tooltip>
  );
}
