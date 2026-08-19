import { describe, expect, it } from "vitest";
import {
  EMPTY_INTERVIEW_ANSWERS,
  draftFromInterview,
  mergeSuggestedAnswers,
} from "@/features/agents/interview-model";

/*
Seven questions are asked, and what the author answered has to arrive.

The last one — what must never happen — is the reason the set works with this
audience: somebody in marketing cannot draft a security policy and answers
"never send to the whole base without a review" without hesitating (FU-07). It
was collected and dropped, which made the interview ask its most valuable
question for nothing.
*/

const TRANSLATED = { tools: ["crm.lookup"], steps: [] };

const ANSWERS = {
  trigger: "Quando chega um chamado em suporte@.",
  mustKnow: "Quem é o cliente e qual o plano dele.",
  steps: "Procuro o cliente, procuro o artigo, respondo.",
  goesWrong: "Às vezes o e-mail não bate com nenhuma conta.",
  notDecide: "Reembolso.",
  closing: "Termina quando a resposta sai.",
  neverDo: "Nunca responder sem citar o artigo.",
};

describe("what an interview leaves behind", () => {
  it("carries what must never happen into the instruction, labelled", () => {
    const draft = draftFromInterview(ANSWERS, TRANSLATED, "pt-BR");

    expect(draft.instructions).toContain("Nunca responder sem citar o artigo.");
    // Labelled rather than appended as a sentence: the block is what makes it
    // legible as a limit rather than as one more paragraph of prose.
    expect(draft.instructions).toContain("Nunca");
  });

  it("carries what the agent must know before acting", () => {
    const draft = draftFromInterview(ANSWERS, TRANSLATED, "pt-BR");

    expect(draft.instructions).toContain("Quem é o cliente e qual o plano dele.");
  });

  /*
  How the work finishes is not the exception that ends it.

  "Quando parar" is where a step's exception goes, and the platform tells the
  model that reaching it means giving up. Filed there, "termina quando a
  resposta sai" makes an agent stop at the moment it succeeds — which is the
  failure this very question exists to describe, turned into the failure it
  causes.
  */
  it("does not file how the work finishes as a reason to give up", () => {
    const draft = draftFromInterview(ANSWERS, TRANSLATED, "pt-BR");

    const text = draft.instructions ?? "";

    // Both are present, and the finishing condition sits above the label that
    // means "give up here" rather than under it. Asserted by position because
    // the payload is one string: the label is what tells the model how to read
    // the sentence beneath it.
    expect(text).toContain("Termina quando a resposta sai.");
    expect(text.indexOf("Termina quando a resposta sai.")).toBeLessThan(
      text.indexOf("Quando parar"),
    );
  });

  // An agent that starts itself because a wizard defaulted is the worst
  // default this product could have, so the answer about when it starts sets
  // no trigger. It is configuration and its home is the field, not the prompt.
  it("sets no trigger from the answer about when it starts", () => {
    const draft = draftFromInterview(ANSWERS, TRANSLATED, "pt-BR");

    expect(draft.triggers).toEqual([]);
  });

  it("uses suggestions to fill blanks without overwriting the author", () => {
    const got = mergeSuggestedAnswers(
      { ...EMPTY_INTERVIEW_ANSWERS, steps: "Eu já corrigi os passos." },
      {
        trigger: "Quando chega um alerta.",
        mustKnow: "Métricas.",
        steps: "Passos sugeridos pelo modelo.",
        goesWrong: "",
        notDecide: "Acionar alguém.",
        closing: "Resumo pronto.",
        neverDo: "Fechar incidente.",
      },
    );

    expect(got.trigger).toBe("Quando chega um alerta.");
    expect(got.steps).toBe("Eu já corrigi os passos.");
  });
});
