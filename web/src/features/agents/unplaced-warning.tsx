import { useTranslation } from "react-i18next";
import { TriangleAlert } from "lucide-react";

/**
 * Granted tools no step reaches. Said where the steps are edited, beside the
 * other warnings about the definition, because placing a tool is done here.
 */
export function UnplacedWarning({ tools }: { tools: string[] }) {
  const { t } = useTranslation();
  if (tools.length === 0) return null;

  return (
    <p className="flex items-start gap-1.5 rounded-md bg-warning-surface px-2.5 py-2 text-2xs text-warning">
      <TriangleAlert className="mt-px size-3.5 shrink-0" aria-hidden />
      {t("agents.unplaced", { count: tools.length, tools: tools.join(", ") })}
    </p>
  );
}
