import { useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { Stamp } from "lucide-react";
import { Button } from "@/components/ui/button";
import { eventCycleMessage } from "@/features/agents/event-cycle-warning";
import { CandidateSheet } from "@/features/workbench/candidate-sheet";
import { CandidatesList } from "@/features/workbench/candidates-list";
import { useSealDraft } from "@/features/workbench/candidates-api";
import { currentLocale } from "@/i18n";
import { problemMessage } from "@/lib/api/problem-message";

/** The draft as it is saved, and whether edits wait to be saved: sealing seals the saved one. */
export type Sealing = { etag: string; unsaved: boolean };

/**
 * The candidates tab (NT-018 phase 2, release 2d): sealing the saved draft as a
 * candidate, which no reader of the agent sees and no real run opens on, and the
 * candidates the person may see, each to open on its sealed text.
 */
export function WorkbenchCandidates({ agentId, sealing }: { agentId: string; sealing: Sealing }) {
  const { t } = useTranslation();
  const seal = useSealDraft(agentId);
  const [open, setOpen] = useState("");

  const sealNow = () =>
    seal.mutate(sealing.etag, {
      onSuccess: (sealed) => {
        // Bytes already published seal nothing: that is information about the draft, not success.
        if (sealed.published) toast.info(t("workbench.candidates.alreadyPublished", { version: sealed.version }));
        else toast.success(t("workbench.candidates.sealed", { version: sealed.version }));
        const cycle = eventCycleMessage(sealed.eventCycleWarning, t, currentLocale());
        if (cycle) toast.warning(cycle, { duration: Infinity });
      },
      onError: (error) => toast.error(problemMessage(error, t)),
    });

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <p className="max-w-prose text-sm text-muted-foreground">{t("workbench.candidates.intro")}</p>
        <Button size="sm" variant="outline" onClick={sealNow} disabled={sealing.unsaved || seal.isPending}>
          <Stamp className="size-4" aria-hidden />
          {t("workbench.candidates.seal")}
        </Button>
      </div>
      {sealing.unsaved && <p className="text-xs text-muted-foreground">{t("workbench.candidates.saveFirst")}</p>}
      <CandidatesList agentId={agentId} onOpen={setOpen} />
      <CandidateSheet agentId={agentId} version={open} onClose={() => setOpen("")} />
    </div>
  );
}
