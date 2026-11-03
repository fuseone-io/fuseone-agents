import { useTranslation } from "react-i18next";
import { Mono } from "@/components/shared/mono";
import { formatMicros } from "@/lib/format";

/**
 * A cost the server sent, or a dash that says the reader may not see it.
 *
 * An absent cost is never zero: the server leaves it out when the reader holds
 * no cost:read over the row, and "R$ 0,00" there would read as free.
 */
export function WorkItemCost({ micros }: { micros?: number }) {
  const { t } = useTranslation();
  if (micros !== undefined) return <>{formatMicros(micros)}</>;
  return (
    <Mono dim>
      <span aria-hidden>—</span>
      <span className="sr-only">{t("workItems.costHidden")}</span>
    </Mono>
  );
}
