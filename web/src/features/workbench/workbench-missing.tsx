import { useTranslation } from "react-i18next";
import { CircleCheck, CircleDashed } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import type { AgentDraft } from "@/lib/api/client";

/** What publication still needs and the draft lacks, as the server lists it. */
export function WorkbenchMissing({ missing }: { missing: AgentDraft["missing"] }) {
  const { t } = useTranslation();

  if (missing.length === 0) {
    return (
      <p className="flex items-center gap-2 text-sm text-muted-foreground">
        <CircleCheck className="size-4" aria-hidden />
        {t("workbench.complete")}
      </p>
    );
  }
  return (
    <section aria-label={t("workbench.missingTitle")} className="flex flex-wrap items-center gap-2">
      <span className="text-sm font-medium">{t("workbench.missingTitle")}</span>
      {missing.map((item) => (
        <Badge key={item} variant="outline" className="gap-1">
          <CircleDashed className="size-3" aria-hidden />
          {t(`workbench.missing.${item}`)}
        </Badge>
      ))}
    </section>
  );
}
