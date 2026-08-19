import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { TemplateCard } from "@/features/agents/template-card";
import {
  useTemplates,
  type AgentTemplate,
} from "@/features/agents/templates-api";

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
  chosen,
  onChoose,
  onClear,
}: {
  /** The template already picked, so the gallery can say which one it was. */
  chosen?: string;
  onChoose: (template: AgentTemplate) => void;
  onClear: () => void;
}) {
  const { t } = useTranslation();
  const { data: templates, isLoading } = useTemplates();

  if (isLoading) {
    return (
      <div className="grid gap-2.5 [grid-template-columns:repeat(auto-fit,minmax(220px,1fr))]">
        {[0, 1, 2, 3].map((i) => (
          <Skeleton key={i} className="h-32" />
        ))}
      </div>
    );
  }
  if (!templates || templates.length === 0) return null;

  return (
    <section className="flex flex-col gap-3">
      <div className="flex items-baseline gap-2">
        <h2 className="text-sm font-medium">{t("agents.startFrom")}</h2>
        <p className="min-w-0 flex-1 truncate text-xs text-muted-foreground">
          {t("agents.startFromHint")}
        </p>
        {chosen && (
          <Button
            type="button"
            variant="ghost"
            size="sm"
            onClick={onClear}
            className="shrink-0 text-xs font-normal text-muted-foreground"
          >
            {t("agents.clearTemplate")}
          </Button>
        )}
      </div>

      {/* Compare as cards, but let the available width decide how many fit.
          Four forced into a narrow authoring column makes real summaries
          overflow; two on a wide column wastes the space this tab owns. */}
      <div className="grid gap-2.5 [grid-template-columns:repeat(auto-fit,minmax(220px,1fr))]">
        {templates.map((template) => (
          <TemplateCard
            key={template.id}
            template={template}
            chosen={template.id === chosen}
            onChoose={() => onChoose(template)}
          />
        ))}
      </div>
    </section>
  );
}
