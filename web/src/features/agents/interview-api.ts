import { useTranslation } from "react-i18next";
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
 *
 * The language goes with it. An author writing in English is instructed in
 * English: their answers come back quoted in their own words, and wrapping
 * them in an instruction written in another language is a mismatch the model
 * pays for — with "in their own words" the first casualty.
 */
export function useInterview() {
  const { i18n } = useTranslation();

  return useMutation({
    mutationFn: async (answers: InterviewAnswers) =>
      unwrap(
        await api.POST("/agents/interview", {
          params: { query: { locale: i18n.language } },
          body: answers,
        }),
      ),
  });
}
