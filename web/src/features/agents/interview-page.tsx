import { useState } from "react";
import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router-dom";
import { MessagesSquare } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { PageHeader } from "@/components/shared/page-header";
import { InterviewQuestion } from "@/features/agents/interview-question";
import { useInterview } from "@/features/agents/interview-api";
import {
  QUESTIONS,
  draftFromInterview,
} from "@/features/agents/interview-model";

/**
 * The interview: seven questions, one at a time (PRD §6.1).
 *
 * The questions are fixed and in order, and no model chooses what to ask next.
 * An authoring path whose questions varied per run could not be reviewed,
 * reproduced or audited — and what it produces is published.
 *
 * It ends on the editor rather than publishing directly. The read-back there
 * is what the author approves (FU-08), and routing both paths through one
 * editor means there is one way to publish rather than two.
 */
export function InterviewPage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const translate = useInterview();

  const [at, setAt] = useState(0);
  const [answers, setAnswers] = useState<Record<string, string>>({});

  const question = QUESTIONS[at];
  const last = at === QUESTIONS.length - 1;

  const finish = () =>
    translate.mutate(
      {
        mustKnow: answers.mustKnow ?? "",
        steps: answers.steps ?? "",
        goesWrong: answers.goesWrong ?? "",
        notDecide: answers.notDecide ?? "",
      },
      {
        onSuccess: (draft) => {
          // Handed to the editor rather than published: nothing reaches an
          // agent registry without a person having read it back first.
          sessionStorage.setItem(
            "fuseone.draft",
            JSON.stringify(draftFromInterview(answers, draft)),
          );
          navigate("/agents/new?from=interview");
        },
        onError: (e) =>
          toast.error(
            e instanceof Error ? e.message : t("interview.assistantFailed"),
          ),
      },
    );

  if (!question) return null;

  return (
    <>
      <PageHeader
        icon={MessagesSquare}
        title={t("interview.title")}
        description={t("interview.subtitle")}
      />

      <div className="flex max-w-2xl flex-col gap-6">
        <p className="font-mono text-2xs tabular-nums text-muted-foreground">
          {t("interview.progress", { at: at + 1, of: QUESTIONS.length })}
        </p>

        <InterviewQuestion
          question={question.key}
          hint={question.hint}
          value={answers[question.fills] ?? ""}
          onChange={(value) =>
            setAnswers({ ...answers, [question.fills]: value })
          }
        />

        <div className="flex items-center gap-2">
          {at > 0 && (
            <Button variant="outline" onClick={() => setAt(at - 1)}>
              {t("interview.back")}
            </Button>
          )}
          <Button
            disabled={translate.isPending}
            onClick={() => (last ? finish() : setAt(at + 1))}
          >
            {t(last ? "interview.finish" : "interview.next")}
          </Button>
          {/* Skippable, and it says so. An author who cannot answer "what
              usually goes wrong" has told you something about the process,
              and blocking them there loses the other six answers. */}
          {!last && (
            <Button variant="ghost" onClick={() => setAt(at + 1)}>
              {t("interview.skip")}
            </Button>
          )}
        </div>
      </div>
    </>
  );
}
