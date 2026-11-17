import { useTranslation } from "react-i18next";
import { Mono } from "@/components/shared/mono";
import { useTurns } from "@/features/workbench/conversation-api";
import { formatMicros } from "@/lib/format";

/**
 * What authoring holds today against its ceiling, installation-wide, as the
 * server judges a turn's reservation: in the header, so the author sees the
 * day's room before writing. Nothing while the assistant is off. The words go
 * on a phone, where the header holds its actions; the figures stay.
 */
export function AuthoringToday({ agentId }: { agentId: string }) {
  const { t } = useTranslation();
  const day = useTurns(agentId).data;
  if (day?.spentMicros === undefined || day.dailyMicros === undefined) return null;

  return (
    <Mono className="text-xs whitespace-nowrap">
      <span className="hidden md:inline">{t("workbench.authoringToday")} </span>
      {t("workbench.authoringOf", { spent: formatMicros(day.spentMicros), daily: formatMicros(day.dailyMicros) })}
    </Mono>
  );
}
