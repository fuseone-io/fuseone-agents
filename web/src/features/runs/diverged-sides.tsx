import { useTranslation } from "react-i18next";
import { divergedKey, type ReplayDivergence } from "@/features/runs/diverged-key";

/**
 * How each side of a comparison left its world, when that is why a case is not comparable
 * (NT-018 phase 5). Nothing for a side that followed it.
 */
export function DivergedSides({ was, now }: { was?: ReplayDivergence; now?: ReplayDivergence }) {
  const { t } = useTranslation();
  const sides = [
    { label: "comparison.divergedWas", d: was },
    { label: "comparison.divergedNow", d: now },
  ].filter((side) => side.d !== undefined);
  if (sides.length === 0) return null;
  return (
    <div className="mt-1 flex flex-col gap-0.5">
      {sides.map(({ label, d }) => (
        <p key={label} className="text-2xs text-muted-foreground">
          {t(label, { how: t(divergedKey(d!), { tool: d!.tool, position: d!.position }) })}
        </p>
      ))}
    </div>
  );
}
