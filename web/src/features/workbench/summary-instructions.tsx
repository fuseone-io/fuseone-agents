import { useTranslation } from "react-i18next";
import { InstructionsDiffView } from "@/features/agents/instructions-diff-view";
import { InstructionsRead } from "@/features/agents/instructions-read";

/**
 * The instructions, read-only (NT-018, contract 3): they change only through
 * a proposal of the conversation its author accepts. Against the published
 * version, as the difference; for an agent never published, as text.
 */
export function SummaryInstructions({ now, published }: { now: string; published?: string }) {
  const { t } = useTranslation();
  if (now === "" && published === undefined) {
    return <p className="text-sm text-muted-foreground">{t("workbench.instructionsEmpty")}</p>;
  }
  if (published === undefined) return <InstructionsRead instructions={now} />;
  return (
    <div className="flex flex-col gap-2">
      <span className="text-xs font-medium text-muted-foreground">{t("workbench.againstPublished")}</span>
      <InstructionsDiffView was={published} now={now} />
    </div>
  );
}
