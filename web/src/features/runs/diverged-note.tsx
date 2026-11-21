import { useTranslation } from "react-i18next";
import { divergedKey, type ReplayDivergence } from "@/features/runs/diverged-key";

/** How a replayed case left the world it was recorded with, in words (NT-018 phase 5). */
export function DivergedNote({ divergence }: { divergence: ReplayDivergence }) {
  const { t } = useTranslation();
  return (
    <p className="mt-1 text-2xs text-muted-foreground">
      {t(divergedKey(divergence), { tool: divergence.tool, position: divergence.position })}
    </p>
  );
}
