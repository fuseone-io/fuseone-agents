import { useTranslation } from "react-i18next";
import { Mono } from "@/components/shared/mono";
import { cn } from "@/lib/utils";
import { useTurns } from "@/features/workbench/conversation-api";
import { formatMicros } from "@/lib/format";

/**
 * What authoring holds today against its ceiling, installation-wide, as the
 * server judges a turn's reservation: in the header, so the author sees the
 * day's room before writing. Nothing while the assistant is off. On a phone
 * the header has no room for it, and it sits in the conversation's own header.
 */
export function AuthoringToday({ agentId, className }: { agentId: string; className?: string }) {
  const { t } = useTranslation();
  const day = useTurns(agentId).data;
  if (day?.spentMicros === undefined || day.dailyMicros === undefined) return null;

  return (
    <Mono className={cn("text-xs whitespace-nowrap", className)}>
      <span className="hidden md:inline">{t("workbench.authoringToday")} </span>
      {t("workbench.authoringOf", { spent: formatMicros(day.spentMicros), daily: formatMicros(day.dailyMicros) })}
    </Mono>
  );
}
