import { useState } from "react";
import { useTranslation } from "react-i18next";
import { BookmarkPlus } from "lucide-react";
import { Button } from "@/components/ui/button";
import { CorrectionDialog, type CorrectionWording } from "@/features/agents/correction-dialog";
import { rehearsalCase } from "@/features/workbench/rehearsal-correction";
import type { Run, Step } from "@/lib/api/client";

// Kept, the case leaves the conversation: erasing it no longer reaches the case (D2).
const KEEPING: CorrectionWording = {
  title: "workbench.cases.keepTitle",
  record: "workbench.cases.keepConfirm",
  warning: "workbench.cases.keepWarning",
};

/**
 * Keeps an ended rehearsal as a case, with the world its author answered for (NT-018 phase
 * 5b). The server judges whether every call was answered and says why when not.
 */
export function RehearsalKeep({ agentId, run, steps }: { agentId: string; run: Run; steps: Step[] }) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);

  return (
    <>
      <Button size="sm" variant="outline" onClick={() => setOpen(true)}>
        <BookmarkPlus className="size-4" aria-hidden />
        {t("workbench.cases.keep")}
      </Button>
      {open && (
        <CorrectionDialog agentId={agentId} entry={rehearsalCase(run, steps)} wording={KEEPING} onClose={() => setOpen(false)} />
      )}
    </>
  );
}
