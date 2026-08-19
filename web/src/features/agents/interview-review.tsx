import { useTranslation } from "react-i18next";
import { FileCheck2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import {
  QUESTIONS,
  filledAnswers,
  type InterviewAnswerKey,
  type InterviewAnswersState,
} from "@/features/agents/interview-model";

const MAX_FIELD_CHARS = 4_000;

export function InterviewReview({
  answers,
  isPending,
  onChange,
  onFinish,
}: {
  answers: InterviewAnswersState;
  isPending: boolean;
  onChange: (field: InterviewAnswerKey, value: string) => void;
  onFinish: () => void;
}) {
  const { t } = useTranslation();
  const missing = QUESTIONS.length - filledAnswers(answers).length;

  return (
    <section className="rounded-xl border bg-card shadow-sm">
      <div className="flex items-center gap-3 border-b border-border-subtle px-4 py-3">
        <span className="flex size-8 shrink-0 items-center justify-center rounded-full bg-primary/10 text-primary">
          <FileCheck2 className="size-4" aria-hidden />
        </span>
        <div className="min-w-0">
          <h2 className="text-sm font-semibold">{t("interview.review")}</h2>
          <p className="text-xs text-muted-foreground">
            {t("interview.reviewHint")}
          </p>
        </div>
      </div>

      <div className="grid gap-4 p-4 md:grid-cols-2">
        {QUESTIONS.map((question) => (
          <div key={question.fills} className="flex min-w-0 flex-col gap-2">
            <Label htmlFor={`interview-${question.fills}`}>
              {t(question.key)}
            </Label>
            <p className="min-h-8 text-xs leading-4 text-muted-foreground">
              {t(question.hint)}
            </p>
            <Textarea
              id={`interview-${question.fills}`}
              value={answers[question.fills]}
              maxLength={MAX_FIELD_CHARS}
              onChange={(event) =>
                onChange(question.fills, event.target.value)
              }
              className="min-h-[112px] resize-y text-sm leading-6"
            />
          </div>
        ))}
      </div>

      <div className="flex flex-wrap items-center justify-between gap-3 border-t border-border-subtle px-4 py-3">
        <p className="text-xs text-muted-foreground">
          {missing === 0
            ? t("interview.ready")
            : t("interview.missingCount", { count: missing })}
        </p>
        <Button onClick={onFinish} disabled={isPending}>
          {t("interview.finish")}
        </Button>
      </div>
    </section>
  );
}
