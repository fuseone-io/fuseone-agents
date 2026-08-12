import { useMutation } from "@tanstack/react-query";
import { api, unwrap } from "@/lib/api/client";
import type { components } from "@/lib/api/schema.gen";

export type InterviewAnswers = components["schemas"]["InterviewAnswers"];
export type InterviewDraft = components["schemas"]["InterviewDraft"];

/**
 * The prose half of the interview, translated by the model Administração
 * points at. The other half — a schedule, a name, an area — never leaves the
 * browser: sending a fact to a model to be understood would spend money to
 * make a certainty less certain.
 */
export function useInterview() {
  return useMutation({
    mutationFn: async (answers: InterviewAnswers) =>
      unwrap(await api.POST("/agents/interview", { body: answers })),
  });
}
