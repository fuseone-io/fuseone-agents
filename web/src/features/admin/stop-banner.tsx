import { useTranslation } from "react-i18next";
import { OctagonX } from "lucide-react";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { StartButton } from "@/features/admin/start-button";
import { useStops } from "@/features/admin/stops-api";

/**
 * What is stopped, on every screen (PRD FO-06).
 *
 * Above the content rather than inside a panel: the question it answers is
 * "why is nothing running", and somebody asking that is looking at a screen
 * of stale numbers, not at the administration area. A stop nobody can see
 * from where they are standing gets diagnosed as an outage.
 */
export function StopBanner() {
  const { t } = useTranslation();
  const { data: stops } = useStops();

  if (!stops || stops.length === 0) return null;

  return (
    <div className="flex flex-col gap-2">
      {stops.map((stop) => (
        <Alert key={`${stop.level}-${stop.scope?.area}-${stop.agentId}`} variant="destructive">
          <OctagonX aria-hidden className="size-4" />
          <AlertTitle>{t(`stops.stopped.${stop.level}`, target(stop))}</AlertTitle>
          <AlertDescription className="flex items-center justify-between gap-4">
            <span>
              {stop.reason}
              {stop.by && ` — ${t("stops.by", { who: stop.by })}`}
            </span>
            <StartButton stop={stop} />
          </AlertDescription>
        </Alert>
      ))}
    </div>
  );
}

/** What the sentence names, for a stop of each level. */
function target(stop: { scope?: { company: string; area: string }; agentId?: string }) {
  return {
    where: stop.scope?.area || stop.scope?.company || "",
    agent: stop.agentId ?? "",
  };
}
