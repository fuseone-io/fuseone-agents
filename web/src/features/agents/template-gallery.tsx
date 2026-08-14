import { useTranslation } from "react-i18next";
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
      <div className="grid gap-2.5 sm:grid-cols-2 xl:grid-cols-4">
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
          <button
            type="button"
            onClick={onClear}
            className="shrink-0 rounded-md px-2 py-1 text-xs text-muted-foreground hover:bg-muted hover:text-foreground"
          >
            {t("agents.clearTemplate")}
          </button>
        )}
      </div>

      {/* Four across, because there are four and they are a set to compare
          rather than a list to read. Two of them stretched over a full row
          made each one look like a section of the form below it. */}
      <div className="grid gap-2.5 sm:grid-cols-2 xl:grid-cols-4">
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
