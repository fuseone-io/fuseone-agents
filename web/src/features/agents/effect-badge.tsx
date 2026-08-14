import { useTranslation } from "react-i18next";
import { cn } from "@/lib/utils";
import type { Tool } from "@/lib/api/client";

/**
 * What a tool does to the world, as a pill.
 *
 * Coloured, because it is the fact the whole card is about: an author scanning
 * eighty rows finds the writes by their colour long before they read a word,
 * and a catalogue where reads and transfers look alike is one where the
 * dangerous row does not stand out at all.
 *
 * Green for what only looks, amber for what changes something — the same two
 * the run screens already use, so the colour means one thing across the
 * console. Colour never carries it alone: the word is in the pill.
 */
const TONE: Record<string, string> = {
  read: "bg-success-surface text-success",
  write: "bg-warning-surface text-warning",
  destructive: "bg-danger-surface text-danger",
  financial: "bg-danger-surface text-danger",
};

export function EffectBadge({ effect }: { effect: Tool["effect"] }) {
  const { t } = useTranslation();

  return (
    <span
      className={cn(
        // justify-self, because the badge is a grid item: without it the pill
        // stretches to the width of its column and stops reading as a pill.
        "w-fit shrink-0 justify-self-start rounded-full px-2 py-px text-2xs",
        TONE[effect] ?? "bg-muted text-muted-foreground",
      )}
    >
      {t(`agents.effect.${effect}`)}
    </span>
  );
}

/**
 * That this tool brings in text somebody outside wrote.
 *
 * Beside the name rather than in the effect column, because it qualifies the
 * tool and not what it does — and because it is what decides whether a write
 * derived from this answer will be stopped, which is a property of the row a
 * reader should meet before the verdict at the end of it.
 */
export function UntrustedBadge() {
  const { t } = useTranslation();

  return (
    <span className="shrink-0 rounded-full bg-warning-surface px-1.5 text-[10px] leading-4 text-warning">
      {t("agents.bringsOutsideData")}
    </span>
  );
}
