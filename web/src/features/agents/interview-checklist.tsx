import { useTranslation } from "react-i18next";
import { CheckCircle2, CircleDashed } from "lucide-react";
import { Progress } from "@/components/ui/progress";
import {
  QUESTIONS,
  filledAnswers,
  type InterviewAnswersState,
} from "@/features/agents/interview-model";
import { cn } from "@/lib/utils";

export function InterviewChecklist({
  answers,
}: {
  answers: InterviewAnswersState;
}) {
  const { t } = useTranslation();
  const filled = filledAnswers(answers);
  const percent = Math.round((filled.length / QUESTIONS.length) * 100);

  return (
    <aside className="rounded-xl border bg-card p-4 shadow-sm">
      <div className="flex items-start justify-between gap-3">
        <div>
          <h2 className="text-sm font-semibold">{t("interview.checklist")}</h2>
          <p className="mt-1 text-xs text-muted-foreground">
            {t("interview.checklistSummary", {
              count: filled.length,
              total: QUESTIONS.length,
            })}
          </p>
        </div>
        <span className="font-mono text-2xs tabular-nums text-muted-foreground">
          {percent}%
        </span>
      </div>
      <Progress value={percent} className="mt-3 h-1.5" />

      <ul className="mt-4 flex flex-col gap-2">
        {QUESTIONS.map((question) => {
          const done = answers[question.fills].trim() !== "";
          return (
            <li
              key={question.fills}
              className="flex items-start gap-2 rounded-lg border border-border-subtle p-2.5"
            >
              {done ? (
                <CheckCircle2
                  className="mt-px size-4 shrink-0 text-success"
                  aria-hidden
                />
              ) : (
                <CircleDashed
                  className="mt-px size-4 shrink-0 text-muted-foreground"
                  aria-hidden
                />
              )}
              <div className="min-w-0">
                <p
                  className={cn(
                    "text-xs font-medium leading-5",
                    !done && "text-muted-foreground",
                  )}
                >
                  {t(question.key)}
                </p>
                <p className="text-2xs text-muted-foreground">
                  {t(done ? "interview.captured" : "interview.missing")}
                </p>
              </div>
            </li>
          );
        })}
      </ul>
    </aside>
  );
}
