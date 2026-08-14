import { useTranslation } from "react-i18next";
import { TriangleAlert } from "lucide-react";

/**
 * What the drawing allows and the words never mention.
 *
 * Said where the process is edited rather than in a toast: it is a fact about
 * the definition somebody is about to publish, and it stays true until one of
 * the two halves changes.
 */
export function DriftWarning({ tools }: { tools: string[] }) {
  const { t } = useTranslation();
  if (tools.length === 0) return null;

  return (
    <p className="flex items-start gap-1.5 rounded-md bg-warning-surface px-2.5 py-2 text-2xs text-warning">
      <TriangleAlert className="mt-px size-3.5 shrink-0" aria-hidden />
      {t("agents.undescribed", {
        count: tools.length,
        tools: tools.join(", "),
      })}
    </p>
  );
}
