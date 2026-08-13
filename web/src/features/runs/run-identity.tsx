import { Trans, useTranslation } from "react-i18next";
import { Check, Copy } from "lucide-react";
import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Separator } from "@/components/ui/separator";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { Mono } from "@/components/shared/mono";
import { StateDot } from "@/components/shared/state-dot";
import { RunNowDialog } from "@/features/agents/run-now-dialog";
import { VerifyButton } from "@/features/runs/verify-button";
import { ExportButton } from "@/features/runs/export-button";
import { AbandonDialog } from "@/features/runs/abandon-dialog";
import { ResumeButton } from "@/features/runs/resume-button";
import { ReplayButton } from "@/features/runs/replay-button";
import { PHASE_LABELS } from "@/features/runs/phase-badge";
import { stateOfPhase } from "@/lib/agent-state";
import { formatInstant } from "@/lib/format";
import type { Phase, Run } from "@/lib/api/client";

// A run in one of these has nowhere left to go.
const ENDED: Phase[] = ["finished", "failed", "unstarted"];

/**
 * Which run this is, in one row.
 *
 * The identifier is copyable because it is what somebody pastes into a ticket,
 * a support thread or a query — and reading a thirty-character id off a screen
 * by hand is how the wrong run ends up in the incident report.
 */
export function RunIdentity({ run, trigger }: { run: Run; trigger?: string }) {
  const { t } = useTranslation();
  return (
    <div className="flex flex-wrap items-start gap-4">
      <div className="min-w-0 flex flex-col gap-1.5">
        <div className="flex items-center gap-2.5">
          <h1 className="text-2xl font-medium tracking-display">
            {run.agentId}
          </h1>
          <span className="inline-flex h-6 items-center gap-1.5 rounded-pill bg-muted px-2.5 text-xs font-medium">
            <StateDot state={stateOfPhase(run.phase)} />
            {t(PHASE_LABELS[run.phase])}
          </span>
        </div>

        <div className="flex flex-wrap items-center gap-2">
          <Mono dim>{run.runId}</Mono>
          <CopyId value={run.runId} />
          <Separator orientation="vertical" className="!h-3" />
          <span className="text-xs text-muted-foreground">
            <Trans
              i18nKey="runs.identityLine"
              values={{
                version: run.versionId.slice(0, 9),
                trigger: trigger ? t("runs.identityTrigger", { trigger }) : "",
                startedAt: formatInstant(run.startedAt),
                onBehalfOf: run.onBehalfOf
                  ? t("runs.onBehalfOf", { who: run.onBehalfOf })
                  : "",
              }}
              components={{ v: <Mono dim /> }}
            />
          </span>
        </div>
      </div>

      <div className="ml-auto flex shrink-0 items-center gap-2">
        {/* A new run of the same agent, on the version published now — not a
            replay. Replaying would re-execute effects that already happened
            against systems that already changed. */}
        <RunNowDialog agentId={run.agentId} agentName={run.agentId} />
        {/* Only while there is a run to end. Abandoning a finished one is a
            request the server refuses, and offering it is a button that
            teaches people the console does not know what it is showing. */}
        {run.phase === "parked" && <ResumeButton runId={run.runId} />}
        {!ENDED.includes(run.phase) && <AbandonDialog runId={run.runId} />}
        <VerifyButton runId={run.runId} />
        {/* Beside verification, because they are the two halves of one
            question: that the steps were not edited, and that they were the
            answer the rules actually give. */}
        <ReplayButton runId={run.runId} />
        <ExportButton runId={run.runId} />
      </div>
    </div>
  );
}

function CopyId({ value }: { value: string }) {
  const { t } = useTranslation();
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
          {copied ? (
            <Check className="size-3.5" />
          ) : (
            <Copy className="size-3.5" />
          )}
          <span className="sr-only">{t("runs.copyRunId")}</span>
        </Button>
      </TooltipTrigger>
      <TooltipContent>
        {copied ? "Copiado" : "Copiar identificador"}
      </TooltipContent>
    </Tooltip>
  );
}
