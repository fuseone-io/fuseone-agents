import { useTranslation } from "react-i18next";
import { Textarea } from "@/components/ui/textarea";

/**
 * One question, alone on the screen.
 *
 * Alone on purpose: seven fields on one form is the notation the interview
 * exists to remove, and somebody answering "what usually goes wrong?" should
 * be thinking about their process rather than about the six boxes below it.
 */
export function InterviewQuestion({
  question,
  hint,
  value,
  onChange,
}: {
  question: string;
  hint?: string;
  value: string;
  onChange: (value: string) => void;
}) {
  const { t } = useTranslation();

  return (
    <div className="flex flex-col gap-3">
      <h2 className="text-xl font-medium tracking-display">{t(question)}</h2>
      {hint && <p className="text-sm text-muted-foreground">{t(hint)}</p>}
      <Textarea
        value={value}
        onChange={(e) => onChange(e.target.value)}
        rows={5}
        className="text-base"
        autoFocus
      />
    </div>
  );
}
