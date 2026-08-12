import { useTranslation } from "react-i18next";
import { Mono } from "@/components/shared/mono";
import { VerdictBadge } from "@/features/agents/simulation-verdict";
import type { SimulationAct } from "@/features/agents/simulation-api";

/**
 * One thing the agent proposed, and what the Gate did about it.
 *
 * The rule is always shown, never only the verdict: "blocked by policy" tells
 * an author nothing about what to change, and the whole reason to simulate is
 * to find out what to change (PRD AU-10).
 */
export function ActRow({ act }: { act: SimulationAct }) {
  const { t } = useTranslation();

  return (
    <li className="flex flex-wrap items-center gap-2 border-t px-4 py-2 first:border-t-0">
      {act.step && (
        <span className="text-xs text-muted-foreground">{act.step}</span>
      )}
      <Mono className="text-xs">{act.tool}</Mono>
      <span className="text-xs text-muted-foreground">{t(effectKey(act))}</span>

      <VerdictBadge verdict={act.verdict} />

      {/* Which rule, and which authored policy when one decided. */}
      {act.rule && (
        <Mono className="text-2xs text-muted-foreground">
          {act.policy ?? act.rule}
        </Mono>
      )}

      <span className="ml-auto text-xs text-muted-foreground">
        {act.reached
          ? t("simulation.wouldHaveCalled")
          : t("simulation.stoppedHere")}
      </span>
    </li>
  );
}

const EFFECTS: Record<string, string> = {
  read: "simulation.effectRead",
  write: "simulation.effectWrite",
  destructive: "simulation.effectDestructive",
  financial: "simulation.effectFinancial",
};

function effectKey(act: SimulationAct): string {
  return EFFECTS[act.effect] ?? "simulation.effectUnknown";
}
