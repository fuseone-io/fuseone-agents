import { useState } from "react";
import { useTranslation } from "react-i18next";
import { CandidateSheet } from "@/features/workbench/candidate-sheet";
import { CandidatesList } from "@/features/workbench/candidates-list";

/**
 * The candidates tab (NT-018 phase 2, release 2d): the candidates the person may see,
 * each to open on its sealed text, which no reader of the agent sees and no real run
 * opens on. Sealing is the header's primary action (#454), so it is reached from every tab.
 */
export function WorkbenchCandidates({ agentId, unsaved }: { agentId: string; unsaved: boolean }) {
  const { t } = useTranslation();
  const [open, setOpen] = useState("");

  return (
    <div className="flex flex-col gap-4">
      <p className="max-w-prose text-sm text-muted-foreground">{t("workbench.candidates.intro")}</p>
      {unsaved && <p className="text-xs text-muted-foreground">{t("workbench.candidates.saveFirst")}</p>}
      <CandidatesList agentId={agentId} onOpen={setOpen} />
      <CandidateSheet agentId={agentId} version={open} onClose={() => setOpen("")} />
    </div>
  );
}
