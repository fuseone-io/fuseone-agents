import { useTranslation } from "react-i18next";
import { TriangleAlert } from "lucide-react";
import { Badge } from "@/components/ui/badge";

/**
 * Tools the draft grants and no step reaches. A run of an agent with steps is
 * offered only what its steps reach, so it never receives these, and
 * publishing from the workbench is refused until each is placed or removed.
 */
export function WorkbenchUnplaced({ tools }: { tools: string[] }) {
  const { t } = useTranslation();
  return (
    <section aria-label={t("workbench.unplacedTitle")} className="flex flex-col gap-2">
      <p className="flex items-center gap-2 text-sm font-medium">
        <TriangleAlert className="size-4 text-warning" aria-hidden />
        {t("workbench.unplacedTitle")}
      </p>
      <p className="text-xs text-muted-foreground">{t("workbench.unplacedHint")}</p>
      <div className="flex flex-wrap gap-2">
        {tools.map((tool) => (
          <Badge key={tool} variant="outline" className="font-mono">
            {tool}
          </Badge>
        ))}
      </div>
    </section>
  );
}
