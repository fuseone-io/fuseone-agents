import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";
import { Mono } from "@/components/shared/mono";

/**
 * The recorded run this one was opened from, when it was (#16).
 *
 * The id and nothing else, which is all the trail carries: the source's
 * approvals and outcome are its own, and the link is how somebody reads them.
 */
export function RunSourceLine({ sourceRunId }: { sourceRunId?: string }) {
  const { t } = useTranslation();
  if (!sourceRunId) return null;
  return (
    <span className="flex items-center gap-1.5 text-xs text-muted-foreground">
      {t("runs.openedFrom")}
      <Link
        to={`/runs/${sourceRunId}`}
        className="underline-offset-2 hover:underline"
      >
        <Mono>{sourceRunId}</Mono>
      </Link>
    </span>
  );
}
