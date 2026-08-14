import { useTranslation } from "react-i18next";

/**
 * Exactly what leaves, and the reason this view is not optional.
 *
 * It is the proof the editor invents nothing: labels are plain lines, and
 * anything the writing view drew as a chip is a bare identifier here. It is
 * also what somebody copies at three in the morning to work out what the
 * agent was actually told.
 */
export function InstructionsPayload({ instructions }: { instructions: string }) {
  const { t } = useTranslation();

  return (
    <div className="flex flex-col gap-2">
      <pre className="max-w-[78ch] rounded-lg bg-muted p-4 font-mono text-xs/[1.75] whitespace-pre-wrap">
        {instructions.trim() || t("agents.nothingWritten")}
      </pre>
      <p className="text-2xs text-muted-foreground">{t("agents.payloadIsThis")}</p>
    </div>
  );
}
