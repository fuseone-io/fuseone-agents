import { useTranslation } from "react-i18next";
import { Mono } from "@/components/shared/mono";
import { WorkItemRefLink } from "@/features/work-items/work-item-ref-link";
import type { WorkItemRun } from "@/lib/api/client";

const LINK_LABELS: Record<WorkItemRun["link"], string> = {
  claim: "workItems.linkClaim",
  alias: "workItems.linkAlias",
  event: "workItems.linkEvent",
  github_delivery: "workItems.linkGitHubDelivery",
};

/**
 * The run's other card, when it has one: the card it claimed, on a run here
 * by its inheritance; the card it inherited, on a run here by its claim.
 * Named only when it is not this trail's own card.
 */
function OtherAttribution({ run, card }: { run: WorkItemRun; card: string }) {
  const { t } = useTranslation();
  const [key, ref] = run.claimedRef
    ? ["workItems.alsoClaimed", run.claimedRef]
    : ["workItems.alsoInherited", run.inheritedRef ?? ""];
  if (!ref || ref === card) return null;
  return (
    <span className="text-xs text-muted-foreground">
      <WorkItemRefLink reference={ref} label={t(key, { ref })} />
    </span>
  );
}

/**
 * How a run is on the trail: it claimed the card or a pull request linked to
 * it, or it inherited one of them from the event or the GitHub delivery that
 * opened it (#204). A reference other than the card is named, so the reader
 * sees which.
 */
export function WorkItemRunLink({
  run,
  card,
}: {
  run: WorkItemRun;
  card: string;
}) {
  const { t } = useTranslation();
  return (
    <span className="flex flex-wrap items-center gap-2">
      {t(LINK_LABELS[run.link])}
      {run.ref !== card && <Mono>{run.ref}</Mono>}
      <OtherAttribution run={run} card={card} />
    </span>
  );
}
