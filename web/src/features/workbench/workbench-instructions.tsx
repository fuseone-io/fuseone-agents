import { useTranslation } from "react-i18next";
import { ScrollText } from "lucide-react";
import { Section } from "@/features/policies/section";
import { InstructionsDiffView } from "@/features/agents/instructions-diff-view";
import { InstructionsRead } from "@/features/agents/instructions-read";

/**
 * The instructions, read-only (NT-018, contract 3): they change only through an
 * accepted proposal of the conversation, and this panel has none. Against the
 * published version, as the difference; for an agent never published, as text.
 */
export function WorkbenchInstructions({
  now,
  published,
}: {
  now: string;
  published?: string;
}) {
  const { t } = useTranslation();

  return (
    <Section icon={ScrollText} title={t("workbench.instructionsTitle")} hint={t("workbench.instructionsNote")}>
      {now === "" && published === undefined ? (
        <p className="text-sm text-muted-foreground">{t("workbench.instructionsEmpty")}</p>
      ) : published === undefined ? (
        <InstructionsRead instructions={now} />
      ) : (
        <div className="flex flex-col gap-2">
          <span className="text-xs font-medium text-muted-foreground">{t("workbench.againstPublished")}</span>
          <InstructionsDiffView was={published} now={now} />
        </div>
      )}
    </Section>
  );
}
