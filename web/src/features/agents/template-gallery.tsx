import { useTranslation } from "react-i18next";
import { ArrowRight, ShieldAlert } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { useTemplates, type AgentTemplate } from "@/features/agents/templates-api";

/**
 * Four shapes somebody can start from, instead of an empty page (PRD FU-16).
 *
 * Above the blank form rather than instead of it: starting from nothing is
 * still legitimate, and a gallery that blocked the form would make an author
 * pick a template they do not want in order to delete it.
 *
 * A template names no tools. It says what it needs to reach in the author's
 * own language, and the pack is chosen below from what this installation has
 * actually connected.
 */
export function TemplateGallery({
  onChoose,
}: {
  onChoose: (template: AgentTemplate) => void;
}) {
  const { t } = useTranslation();
  const { data: templates, isLoading } = useTemplates();

  if (isLoading) {
    return (
      <div className="grid gap-3 sm:grid-cols-2">
        <Skeleton className="h-28" />
        <Skeleton className="h-28" />
      </div>
    );
  }
  if (!templates || templates.length === 0) return null;

  return (
    <section className="flex flex-col gap-3">
      <div>
        <h2 className="text-sm font-medium">{t("agents.startFrom")}</h2>
        <p className="text-xs text-muted-foreground">
          {t("agents.startFromHint")}
        </p>
      </div>

      <div className="grid gap-3 sm:grid-cols-2">
        {templates.map((template) => (
          <Card key={template.id} template={template} onChoose={onChoose} />
        ))}
      </div>
    </section>
  );
}

function Card({
  template,
  onChoose,
}: {
  template: AgentTemplate;
  onChoose: (template: AgentTemplate) => void;
}) {
  const { t } = useTranslation();

  return (
    <article className="flex flex-col gap-2 rounded-xl border bg-card p-3 shadow-sm">
      <div>
        <h3 className="text-sm font-medium">{template.name}</h3>
        <p className="mt-0.5 text-xs text-muted-foreground">
          {template.summary}
        </p>
      </div>

      {/* What it needs to reach, not which tools: the author picks those from
          what the Curator connected, and a template naming one would be broken
          in every installation that calls it something else. */}
      <ul className="flex flex-col gap-1">
        {template.needs.map((need) => (
          <li
            key={need}
            className="flex items-start gap-1.5 text-2xs text-muted-foreground"
          >
            <ShieldAlert className="mt-px size-3 shrink-0" aria-hidden />
            {need}
          </li>
        ))}
      </ul>

      <Button
        variant="outline"
        size="sm"
        className="mt-auto self-start"
        onClick={() => onChoose(template)}
      >
        {t("agents.startFromThis")}
        <ArrowRight className="size-3.5" aria-hidden />
      </Button>
    </article>
  );
}
