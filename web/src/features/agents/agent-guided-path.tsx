import { CheckCircle2, CircleDashed, ListChecks } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";
import { Button } from "@/components/ui/button";
import {
  guidedAgentProgress,
  type GuidedAgentStep,
} from "@/features/agents/agent-guided-path-model";
import type { EditorTab } from "@/features/agents/editor-tabs";
import { cn } from "@/lib/utils";

export function AgentGuidedPath({
  steps,
  onOpen,
  titleKey = "agents.guideTitle",
  subtitleKey = "agents.guideSubtitle",
  progressKey = "agents.guideProgress",
}: {
  steps: GuidedAgentStep[];
  onOpen?: (tab: EditorTab) => void;
  titleKey?: string;
  subtitleKey?: string;
  progressKey?: string;
}) {
  const { t } = useTranslation();
  const progress = guidedAgentProgress(steps);
  const next = progress.next;
  const nextTab = next?.tab;

  return (
    <section
      data-testid="agent-guided-path"
      className="border-b border-border bg-muted/30 px-4 py-3"
    >
      <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
        <div className="flex min-w-0 flex-1 items-center gap-2">
          <ListChecks
            className="size-4 shrink-0 text-text-accent"
            aria-hidden
          />
          <div className="min-w-0">
            <h2 className="truncate text-sm font-semibold">
              {t(titleKey)}
            </h2>
            <p className="truncate text-2xs text-muted-foreground">
              {t(subtitleKey)}
            </p>
          </div>
        </div>

        <span className="rounded-pill bg-surface-accent px-2 py-1 text-2xs text-text-accent">
          {t(progressKey, progress)}
        </span>
        {next?.to && (
          <Button
            type="button"
            variant="outline"
            size="sm"
            className="h-8"
            asChild
          >
            <Link to={next.to}>
              {t("agents.guideContinue", { step: t(next.labelKey) })}
            </Link>
          </Button>
        )}
        {next && nextTab && onOpen && (
          <Button
            type="button"
            variant="outline"
            size="sm"
            className="h-8"
            onClick={() => onOpen(nextTab)}
          >
            {t("agents.guideContinue", { step: t(next.labelKey) })}
          </Button>
        )}
      </div>

      <ol className="mt-3 grid gap-2 [grid-template-columns:repeat(auto-fit,minmax(min(100%,180px),1fr))]">
        {steps.map((step) => (
          <li key={step.id} className="min-w-0">
            <StepControl step={step} onOpen={onOpen} />
          </li>
        ))}
      </ol>
    </section>
  );
}

function StepControl({
  step,
  onOpen,
}: {
  step: GuidedAgentStep;
  onOpen?: (tab: EditorTab) => void;
}) {
  const { t } = useTranslation();
  const className = cn(
    "flex h-full w-full min-w-0 items-start gap-2 rounded-lg border bg-card/60 px-3 py-2 text-left transition-colors",
    step.done && "border-border-subtle bg-transparent",
    (step.to || (step.tab && onOpen)) && "hover:border-primary/50 hover:bg-card",
  );
  const body = (
    <>
      {step.done ? (
        <CheckCircle2
          className="mt-0.5 size-4 shrink-0 text-success"
          aria-hidden
        />
      ) : (
        <CircleDashed
          className="mt-0.5 size-4 shrink-0 text-muted-foreground"
          aria-hidden
        />
      )}
      <span className="min-w-0">
        <span className="flex min-w-0 flex-wrap items-center gap-1.5 text-xs font-medium">
          <span className="min-w-0 break-words">{t(step.labelKey)}</span>
          {step.optional && (
            <span className="rounded-pill bg-muted px-1.5 py-0.5 text-2xs text-muted-foreground">
              {t("agents.guideRecommended")}
            </span>
          )}
        </span>
        <span className="mt-1 block break-words text-2xs leading-snug text-muted-foreground">
          {t(step.bodyKey, step.bodyValues)}
        </span>
      </span>
    </>
  );

  if (step.to) {
    return (
      <Link to={step.to} className={className}>
        {body}
      </Link>
    );
  }
  const tab = step.tab;
  if (tab && onOpen) {
    return (
      <button type="button" onClick={() => onOpen(tab)} className={className}>
        {body}
      </button>
    );
  }
  return <div className={className}>{body}</div>;
}
