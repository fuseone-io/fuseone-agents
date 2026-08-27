import { useTranslation } from "react-i18next";
import { FileText, UserRoundCheck } from "lucide-react";
import { Mono } from "@/components/shared/mono";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { AgentFlow } from "@/features/agents/agent-flow";
import { InstructionsRead } from "@/features/agents/instructions-read";
import { StepReaches } from "@/features/agents/step-reaches";
import type { components } from "@/lib/api/schema.gen";

/**
 * What somebody told the agent to do, exactly as published.
 *
 * Read-only, and it will stay read-only: a specification is changed by
 * publishing a new version, never by editing one that runs already reference.
 * An editable box here would let somebody rewrite the explanation of a run
 * that already happened.
 */
export function AgentDefinition({
  instructions,
  source,
  steps,
  view = "auto",
}: {
  instructions?: string;
  source?: string;
  steps?: components["schemas"]["AgentStep"][];
  view?: "auto" | "instructions" | "steps";
}) {
  const { t } = useTranslation();
  const declared = steps ?? [];
  const showTabs = view === "auto" && declared.length > 0;
  return (
    <section className="flex flex-col gap-3 rounded-xl border border-border bg-card p-4 shadow-sm">
      <div className="flex flex-wrap items-center gap-2">
        <h2 className="text-sm font-medium">{t("agents.definition")}</h2>
        {source && (
          <Mono dim className="truncate">
            {source}
          </Mono>
        )}
        <span className="ml-auto text-xs text-muted-foreground">
          {t("agents.publishedReadOnly")}
        </span>
      </div>

      {/* The toggle appears only where there is a process to draw. An agent
          that declares no steps has one envelope holding its whole pack, and
          a diagram of that would show a single box — teaching a reader it has
          one step when it has none declared. */}
      {view === "steps" ? (
        <StepsOnly steps={declared} />
      ) : view === "instructions" ? (
        <Prose instructions={instructions} steps={declared} />
      ) : showTabs ? (
        <Tabs defaultValue="prose">
          <TabsList className="h-8">
            <TabsTrigger value="prose">{t("agents.asProse")}</TabsTrigger>
            <TabsTrigger value="flow">{t("agents.asFlow")}</TabsTrigger>
          </TabsList>
          <TabsContent value="prose">
            <Prose instructions={instructions} steps={declared} />
          </TabsContent>
          <TabsContent value="flow" className="pt-1">
            <AgentFlow steps={declared} />
          </TabsContent>
        </Tabs>
      ) : (
        <Prose instructions={instructions} steps={declared} />
      )}
    </section>
  );
}

function StepsOnly({ steps }: { steps: components["schemas"]["AgentStep"][] }) {
  const { t } = useTranslation();
  if (steps.length === 0) {
    return (
      <div className="flex min-h-48 flex-col items-center justify-center rounded-lg border border-dashed border-border bg-muted/30 p-8 text-center">
        <p className="max-w-sm text-sm font-medium">
          {t("agents.noFixedSteps")}
        </p>
        <p className="mt-2 max-w-md text-xs text-muted-foreground">
          {t("agents.noFixedStepsHint")}
        </p>
      </div>
    );
  }
  return <AgentFlow steps={steps} />;
}

/**
 * The words, plus the stages the published version declared.
 *
 * Kept as two sections rather than generated prose: the body remains exactly
 * what the author wrote, and the stage list remains exactly what the Gate
 * uses. The read view still has to show both, otherwise changing a stage looks
 * like it never changed the definition.
 *
 * Laid out the way it was written, labels in the margin. The editor gives a
 * prompt its own hierarchy and a version rendered as one paragraph throws it
 * away — at the moment somebody is working out what the agent was told.
 */
function Prose({
  instructions,
  steps,
}: {
  instructions?: string;
  steps: components["schemas"]["AgentStep"][];
}) {
  const { t } = useTranslation();
  return (
    <div className="min-w-0">
      {instructions ? (
        <InstructionsRead instructions={instructions} />
      ) : (
        <p className="flex items-center gap-2 py-6 text-sm text-muted-foreground">
          <FileText className="size-4" aria-hidden />
          {t("agents.publishedWithout")}
        </p>
      )}
      <DeclaredStepsRead steps={steps} />
    </div>
  );
}

function DeclaredStepsRead({
  steps,
}: {
  steps: components["schemas"]["AgentStep"][];
}) {
  const { t } = useTranslation();
  if (steps.length === 0) return null;

  return (
    <section className="mt-4 min-w-0 border-t border-border pt-4">
      <h3 className="mb-2 text-2xs font-medium uppercase tracking-label text-muted-foreground">
        {t("agents.asFlow")}
      </h3>
      <ol className="flex min-w-0 flex-col gap-2">
        {steps.map((step, at) => (
          <li
            key={at}
            className="grid min-w-0 grid-cols-[22px_minmax(0,1fr)] items-start gap-3 border-b border-border-subtle pb-2 last:border-0 last:pb-0"
          >
            <span className="grid size-[22px] place-items-center rounded-pill border border-border bg-muted font-mono text-[11px] tabular-nums text-muted-foreground">
              {at + 1}
            </span>
            <div className="min-w-0">
              <p className="text-sm font-medium text-pretty">
                {step.name || (
                  <span className="italic text-muted-foreground">
                    {t("agents.unnamedStep")}
                  </span>
                )}
              </p>
              <div className="mt-1 flex min-w-0 flex-wrap items-center gap-1.5">
                <StepReaches reaches={step.reaches} />
                {step.stopsWhen && (
                  <span className="inline-flex min-w-0 items-center gap-1.5 text-2xs text-warning">
                    <UserRoundCheck className="size-3.5 shrink-0" aria-hidden />
                    <span className="truncate">
                      {t("agents.stopsWhen", { what: step.stopsWhen })}
                    </span>
                  </span>
                )}
              </div>
            </div>
          </li>
        ))}
      </ol>
    </section>
  );
}
