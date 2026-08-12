import { useTranslation } from "react-i18next";
import { Badge } from "@/components/ui/badge";
import type { SimulationCase } from "@/features/agents/simulation-api";

/** Where a case ended, as a word. Never the colour alone: a chip that read
 *  only red would carry its meaning in a channel a third of readers cannot
 *  use. */
const SETTLED: Record<
  SimulationCase["settled"],
  { label: string; variant: "secondary" | "destructive" | "outline" }
> = {
  finished: { label: "simulation.settledFinished", variant: "secondary" },
  parked: { label: "simulation.settledParked", variant: "destructive" },
  awaiting_approval: { label: "simulation.settledWaiting", variant: "outline" },
  unsettled: { label: "simulation.settledRunning", variant: "outline" },
};

export function SettledBadge({
  settled,
}: {
  settled: SimulationCase["settled"];
}) {
  const { t } = useTranslation();
  const shown = SETTLED[settled] ?? SETTLED.unsettled;
  return <Badge variant={shown.variant}>{t(shown.label)}</Badge>;
}
