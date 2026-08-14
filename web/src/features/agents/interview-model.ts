import type { InterviewDraft } from "@/features/agents/interview-api";
import type { AgentDefinition } from "@/lib/api/client";

/** The seven questions, in the order the PRD asks them (FU-01…07). */
export const QUESTIONS = [
  {
    fills: "trigger",
    key: "interview.whenDoesItStart",
    hint: "interview.whenHint",
  },
  {
    fills: "mustKnow",
    key: "interview.whatMustYouKnow",
    hint: "interview.mustKnowHint",
  },
  {
    fills: "steps",
    key: "interview.whatAreTheSteps",
    hint: "interview.stepsHint",
  },
  {
    fills: "goesWrong",
    key: "interview.whatGoesWrong",
    hint: "interview.goesWrongHint",
  },
  {
    fills: "notDecide",
    key: "interview.whatWouldYouNotDecide",
    hint: "interview.notDecideHint",
  },
  {
    fills: "closing",
    key: "interview.howDoYouKnowItIsDone",
    hint: "interview.closingHint",
  },
  // Last, and the reason the set works with this audience: somebody in
  // marketing cannot draft a security policy, but answers "never send to the
  // whole base without a review" without hesitating (PRD FU-07).
  {
    fills: "neverDo",
    key: "interview.whatMustNeverHappen",
    hint: "interview.neverHint",
  },
] as const;

/**
 * What the answers amount to, before anybody edits them.
 *
 * The prose the author wrote becomes the instructions verbatim. It is what an
 * auditor reads in two years to understand a run, and paraphrasing it would
 * replace the author's words with the platform's.
 */
export function draftFromInterview(
  answers: Record<string, string>,
  translated: InterviewDraft,
): Partial<AgentDefinition> {
  return {
    tools: translated.tools,
    // The assistant reads the author's account of the process and answers
    // with the stages in it. They used to be dropped here, which left the one
    // part of a definition the Gate is meant to obey coming out of an
    // interview about exactly that and going nowhere.
    steps: translated.steps,
    instructions: [answers.steps, answers.goesWrong, answers.closing]
      .filter(Boolean)
      .join("\n\n"),
    // Never a trigger nobody asked for: an agent that starts itself because a
    // wizard defaulted is the worst default this product could have.
    triggers: [],
  };
}
