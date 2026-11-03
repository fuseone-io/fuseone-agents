import { useTranslation } from "react-i18next";
import { currentLocale } from "@/i18n";
import type { WorkItemMeasures } from "@/lib/api/client";

type Unavailable = WorkItemMeasures["unavailable"][number];

/**
 * Measure names and reasons the console knows how to say. A name the server
 * starts sending later is said generically, never dropped and never a crash:
 * the server is the one that knows what it cannot count.
 */
const MEASURES: Record<string, string> = {
  edited: "workItems.measures.unavailableEdited",
  laneTime: "workItems.measures.unavailableLaneTime",
  caughtLater: "workItems.measures.unavailableCaughtLater",
};
const REASONS: Record<string, string> = {
  not_recorded: "workItems.measures.reasonNotRecorded",
  board_only: "workItems.measures.reasonBoardOnly",
};
const UNKNOWN_MEASURE = "workItems.measures.unavailableOther";
const UNKNOWN_REASON = "workItems.measures.reasonOther";

/** Entries that share a reason are said in one sentence, in server order. */
function byReason(entries: Unavailable[]): Map<string, string[]> {
  const groups = new Map<string, string[]>();
  for (const { measure, reason } of entries) {
    groups.set(reason, [...(groups.get(reason) ?? []), measure]);
  }
  return groups;
}

/**
 * What these counts leave out, in words. Never a number and never a zero: a
 * measure nobody recorded is not a measure that came out at nothing.
 */
export function WorkItemUnavailable({ entries }: { entries: Unavailable[] }) {
  const { t } = useTranslation();
  if (entries.length === 0) return null;
  const list = new Intl.ListFormat(currentLocale(), { type: "conjunction" });
  const name = (measure: string, first: boolean) => {
    const said = t(MEASURES[measure] ?? UNKNOWN_MEASURE, { name: measure });
    return first ? said : said.charAt(0).toLowerCase() + said.slice(1);
  };
  return (
    <div
      role="note"
      aria-label={t("workItems.measures.unavailableTitle")}
      className="flex flex-col gap-1 px-4 py-3 text-sm text-muted-foreground"
    >
      <p className="font-medium">{t("workItems.measures.unavailableTitle")}</p>
      <ul className="flex flex-col gap-0.5">
        {[...byReason(entries)].map(([reason, measures]) => (
          <li key={reason}>
            {list.format(measures.map((m, i) => name(m, i === 0)))}
            {": "}
            {t(REASONS[reason] ?? UNKNOWN_REASON, { count: measures.length })}
          </li>
        ))}
      </ul>
    </div>
  );
}
