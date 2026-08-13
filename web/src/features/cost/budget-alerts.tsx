import { useTranslation } from "react-i18next";
import { AlertTriangle, OctagonAlert } from "lucide-react";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Progress } from "@/components/ui/progress";
import { formatMicros } from "@/lib/format";
import { useBudgetAlerts, type BudgetAlert } from "@/features/cost/alerts-api";

/**
 * Warnings before the ceiling, where somebody is looking at the money.
 *
 * Half is a note and four fifths is time to act, so only the last one is
 * styled as a failure. A screen that shouts at 50% is a screen people have
 * stopped reading by the time it matters.
 */
export function BudgetAlerts({
  scope,
}: {
  scope?: { company?: string; area?: string };
}) {
  const { data: alerts } = useBudgetAlerts(scope);
  if (!alerts || alerts.length === 0) return null;

  return (
    <div className="flex flex-col gap-2">
      {alerts.map((alert) => (
        <AlertRow
          key={`${alert.scope.company}/${alert.scope.area}`}
          alert={alert}
        />
      ))}
    </div>
  );
}

function AlertRow({ alert }: { alert: BudgetAlert }) {
  const { t } = useTranslation();
  const share = alert.ceilingMicros
    ? Math.min(100, (alert.spentMicros / alert.ceilingMicros) * 100)
    : 0;
  const exhausted = alert.threshold >= 100;

  return (
    <Alert variant={exhausted ? "destructive" : "default"}>
      {exhausted ? (
        <OctagonAlert aria-hidden className="size-4" />
      ) : (
        <AlertTriangle aria-hidden className="size-4 text-warning" />
      )}
      <AlertTitle>
        {t(exhausted ? "cost.alertExhausted" : "cost.alertApproaching", {
          where: alert.scope.area || alert.scope.company,
          threshold: alert.threshold,
        })}
      </AlertTitle>
      <AlertDescription className="flex flex-col gap-2">
        <span>
          {t("cost.alertSpend", {
            spent: formatMicros(alert.spentMicros),
            ceiling: formatMicros(alert.ceilingMicros),
          })}
        </span>
        <Progress value={share} className="h-1.5" />
      </AlertDescription>
    </Alert>
  );
}
