import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { useInterview } from "@/features/agents/interview-api";
import { problemMessage } from "@/lib/api/problem-message";
import type { components } from "@/lib/api/schema.gen";

type AgentStep = components["schemas"]["AgentStep"];

/**
 * Asking the assistant to read the instructions as stages.
 *
 * Its own hook because it is a different kind of act from arranging cards:
 * this one spends real money at the provider, counts against the assistant's
 * daily ceiling, and appends what it cost to the administrative trail whether
 * or not the answer was usable. Arranging a card costs nothing and is undone
 * by arranging it back.
 */
export function useStepProposal(
  instructions: string,
  onProposed: (steps: AgentStep[]) => void,
) {
  const { t } = useTranslation();
  const propose = useInterview();

  const read = () =>
    propose.mutate(
      { steps: instructions },
      {
        onSuccess: (drafted) => {
          onProposed(drafted.steps);
          toast.success(t("agents.stepsProposed"), {
            description: t("agents.stepsProposedHint"),
          });
        },
        onError: (error) => toast.error(problemMessage(error, t)),
      },
    );

  return { read, reading: propose.isPending };
}
