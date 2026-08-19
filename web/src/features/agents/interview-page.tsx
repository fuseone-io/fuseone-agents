import { useState } from "react";
import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router-dom";
import { MessagesSquare } from "lucide-react";
import { toast } from "sonner";
import { PageHeader } from "@/components/shared/page-header";
import { InterviewCapture } from "@/features/agents/interview-capture";
import { InterviewChecklist } from "@/features/agents/interview-checklist";
import {
  useInterview,
  useInterviewSuggestions,
} from "@/features/agents/interview-api";
import { InterviewReview } from "@/features/agents/interview-review";
import {
  EMPTY_INTERVIEW_ANSWERS,
  draftFromInterview,
  mergeSuggestedAnswers,
  type InterviewAnswerKey,
  type InterviewAnswersState,
} from "@/features/agents/interview-model";
import { problemMessage } from "@/lib/api/problem-message";

/**
 * The interview: a free description resolved into seven fixed fields.
 *
 * The fields are fixed and no model chooses what to ask next.
 * An authoring path whose questions varied per run could not be reviewed,
 * reproduced or audited — and what it produces is published. The free-form
 * capture is ergonomics; the review is the contract.
 *
 * It ends on the editor rather than publishing directly. The read-back there
 * is what the author approves (FU-08), and routing both paths through one
 * editor means there is one way to publish rather than two.
 */
export function InterviewPage() {
  const { t, i18n } = useTranslation();
  const navigate = useNavigate();
  const translate = useInterview();
  const suggest = useInterviewSuggestions();

  const [capture, setCapture] = useState("");
  const [reviewing, setReviewing] = useState(false);
  const [answers, setAnswers] = useState<InterviewAnswersState>(
    EMPTY_INTERVIEW_ANSWERS,
  );

  const review = () => {
    setAnswers((current) => ({
      ...current,
      steps: current.steps.trim() === "" ? capture : current.steps,
    }));
    setReviewing(true);
  };

  const changeAnswer = (field: InterviewAnswerKey, value: string) =>
    setAnswers((current) => ({ ...current, [field]: value }));

  const suggestAnswers = () =>
    suggest.mutate(
      { text: capture },
      {
        onSuccess: (result) => {
          setAnswers((current) =>
            mergeSuggestedAnswers(current, result.answers),
          );
          setReviewing(true);
        },
        onError: (e) => toast.error(problemMessage(e, t)),
      },
    );

  const finish = () =>
    translate.mutate(
      {
        mustKnow: answers.mustKnow ?? "",
        steps: answers.steps ?? "",
        goesWrong: answers.goesWrong ?? "",
        notDecide: answers.notDecide ?? "",
        // The limit the assistant was never told, and it changes what it
        // should answer: a tool the author has just forbidden is a tool they
        // will have to take away again.
        neverDo: answers.neverDo ?? "",
      },
      {
        onSuccess: (draft) => {
          // Handed to the editor rather than published: nothing reaches an
          // agent registry without a person having read it back first.
          sessionStorage.setItem(
            "fuseone.draft",
            JSON.stringify(draftFromInterview(answers, draft, i18n.language)),
          );
          navigate("/agents/new?from=interview");
        },
        onError: (e) => toast.error(problemMessage(e, t)),
      },
    );

  return (
    <>
      <PageHeader
        icon={MessagesSquare}
        title={t("interview.title")}
        description={t("interview.subtitle")}
      />

      <div className="grid max-w-6xl gap-4 xl:grid-cols-[minmax(0,1fr)_320px]">
        <div className="flex min-w-0 flex-col gap-4">
          <InterviewCapture
            value={capture}
            onChange={setCapture}
            onReview={review}
            onSuggest={suggestAnswers}
            suggesting={suggest.isPending}
          />
          {reviewing && (
            <InterviewReview
              answers={answers}
              isPending={translate.isPending}
              onChange={changeAnswer}
              onFinish={finish}
            />
          )}
        </div>
        <InterviewChecklist answers={answers} />
      </div>
    </>
  );
}
