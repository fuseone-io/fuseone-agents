import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { Stamp } from "lucide-react";
import { Button } from "@/components/ui/button";
import { eventCycleMessage } from "@/features/agents/event-cycle-warning";
import { useSealDraft } from "@/features/workbench/candidates-api";
import { currentLocale } from "@/i18n";
import { problemMessage } from "@/lib/api/problem-message";

/**
 * Seals the saved draft as a candidate (NT-018 phase 2): the workbench's primary action
 * once there is nothing to save (#454). The candidate path, seal, rehearse or run it
 * against the cases, then activate, is the way to production; publishing straight is
 * the exception beside it.
 */
export function WorkbenchSeal({ agentId, etag, unsaved }: { agentId: string; etag: string; unsaved: boolean }) {
  const { t } = useTranslation();
  const seal = useSealDraft(agentId);

  const sealNow = () =>
    seal.mutate(etag, {
      onSuccess: (sealed) => {
        // Bytes already published seal nothing: that is information about the draft, not success.
        if (sealed.published) toast.info(t("workbench.candidates.alreadyPublished", { version: sealed.version }));
        else toast.success(t("workbench.candidates.sealed", { version: sealed.version }));
        const cycle = eventCycleMessage(sealed.eventCycleWarning, t, currentLocale());
        if (cycle) toast.warning(cycle, { duration: Infinity });
      },
      onError: (error) => toast.error(problemMessage(error, t)),
    });

  // Saving holds the header's one primary slot while edits wait: the seal shows once nothing does.
  if (unsaved) return null;
  return (
    <Button size="sm" onClick={sealNow} disabled={seal.isPending}>
      <Stamp className="size-4" aria-hidden />
      <span className="sr-only lg:not-sr-only">{t("workbench.candidates.seal")}</span>
    </Button>
  );
}
