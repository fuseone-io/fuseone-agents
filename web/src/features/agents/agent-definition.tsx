import { useTranslation } from "react-i18next";
import { FileText } from "lucide-react";
import { Mono } from "@/components/shared/mono";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { AgentFlow } from "@/features/agents/agent-flow";
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
}: {
  instructions?: string;
  source?: string;
  steps?: components["schemas"]["AgentStep"][];
}) {
  const { t } = useTranslation();
  const declared = steps ?? [];
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
      {declared.length > 0 ? (
        <Tabs defaultValue="prose">
          <TabsList className="h-8">
            <TabsTrigger value="prose">{t("agents.asProse")}</TabsTrigger>
            <TabsTrigger value="flow">{t("agents.asFlow")}</TabsTrigger>
          </TabsList>
          <TabsContent value="prose">
            <Prose instructions={instructions} />
          </TabsContent>
          <TabsContent value="flow" className="pt-1">
            <AgentFlow steps={declared} />
          </TabsContent>
        </Tabs>
      ) : (
        <Prose instructions={instructions} />
      )}
    </section>
  );
}

/**
 * The words, which are what the model reads.
 *
 * Kept apart from the steps deliberately: the prose is the instruction and
 * the steps are what the Gate is meant to obey. Showing them as one document
 * would hide that they are two different things with two different readers.
 */
function Prose({ instructions }: { instructions?: string }) {
  const { t } = useTranslation();
  if (!instructions) {
    return (
      <p className="flex items-center gap-2 py-6 text-sm text-muted-foreground">
        <FileText className="size-4" aria-hidden />
        {t("agents.publishedWithout")}
      </p>
    );
  }
  return (
    <p className="whitespace-pre-wrap text-sm leading-relaxed">{instructions}</p>
  );
}
