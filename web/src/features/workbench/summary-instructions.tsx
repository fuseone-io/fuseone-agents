import { useState } from "react";
import { useTranslation } from "react-i18next";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { diffInstructions } from "@/features/agents/instruction-diff";
import { InstructionsDiffView } from "@/features/agents/instructions-diff-view";
import { InstructionsRead } from "@/features/agents/instructions-read";

type View = "text" | "diff";

/**
 * The instructions, read-only (NT-018, contract 3): they change only through
 * a proposal of the conversation its author accepts. Read-only is never
 * hidden — this is the one place the author reads what the agent is told — so
 * the text is always on the screen. When the draft changed the prose against
 * the published version, the section opens on the difference, and the text is
 * one press away.
 */
export function SummaryInstructions({ now, published }: { now: string; published?: string }) {
  const { t } = useTranslation();
  const [view, setView] = useState<View>("diff");
  if (now === "" && published === undefined) {
    return <p className="text-sm text-muted-foreground">{t("workbench.instructionsEmpty")}</p>;
  }
  const changed =
    published !== undefined && diffInstructions(published, now).some((block) => block.state !== "same");
  if (!changed) return <InstructionsRead instructions={now} />;
  return (
    <div className="flex flex-col gap-2">
      <ToggleGroup
        type="single"
        size="sm"
        variant="outline"
        value={view}
        // Never empty: pressing the active one again would leave no instructions.
        onValueChange={(next) => next && setView(next as View)}
        aria-label={t("workbench.instructionsView")}
        className="self-start"
      >
        <ToggleGroupItem value="diff">{t("workbench.instructionsDiff")}</ToggleGroupItem>
        <ToggleGroupItem value="text">{t("workbench.instructionsText")}</ToggleGroupItem>
      </ToggleGroup>
      {view === "diff" ? (
        <>
          <span className="text-xs font-medium text-muted-foreground">{t("workbench.againstPublished")}</span>
          <InstructionsDiffView was={published} now={now} />
        </>
      ) : (
        <InstructionsRead instructions={now} />
      )}
    </div>
  );
}
