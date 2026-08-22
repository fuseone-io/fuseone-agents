import { CheckCircle2, CircleDashed, ListChecks } from "lucide-react";
import { useTranslation } from "react-i18next";
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
}: {
  steps: GuidedAgentStep[];
  onOpen: (tab: EditorTab) => void;
}) {
  const { t } = useTranslation();
  const progress = guidedAgentProgress(steps);
  const next = progress.next;

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
              {t("agents.guideTitle")}
            </h2>
            <p className="truncate text-2xs text-muted-foreground">
              {t("agents.guideSubtitle")}
            </p>
          </div>
        </div>

        <span className="rounded-pill bg-surface-accent px-2 py-1 text-2xs text-text-accent">
          {t("agents.guideProgress", progress)}
        </span>
        {next && (
          <Button
            type="button"
            variant="outline"
            size="sm"
            className="h-8"
            onClick={() => onOpen(next.tab)}
          >
            {t("agents.guideContinue", { step: t(next.labelKey) })}
          </Button>
        )}
      </div>

      <ol className="mt-3 grid gap-2 [grid-template-columns:repeat(auto-fit,minmax(min(100%,180px),1fr))]">
        {steps.map((step) => (
          <li key={step.id} className="min-w-0">
            <button
              type="button"
              onClick={() => onOpen(step.tab)}
              className={cn(
                "flex h-full w-full min-w-0 items-start gap-2 rounded-lg border bg-card/60 px-3 py-2 text-left transition-colors hover:border-primary/50 hover:bg-card",
                step.done && "border-border-subtle bg-transparent",
              )}
            >
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
                  <span className="min-w-0 break-words">
                    {t(step.labelKey)}
                  </span>
                  {step.optional && (
                    <span className="rounded-pill bg-muted px-1.5 py-0.5 text-2xs text-muted-foreground">
                      {t("agents.guideRecommended")}
                    </span>
                  )}
                </span>
                <span className="mt-1 block break-words text-2xs leading-snug text-muted-foreground">
                  {t(step.bodyKey)}
                </span>
              </span>
            </button>
          </li>
        ))}
      </ol>
    </section>
  );
}
