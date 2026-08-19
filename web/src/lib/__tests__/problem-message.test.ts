import { describe, expect, it } from "vitest";
import { ApiError } from "@/lib/api/client";
import { problemMessage } from "@/lib/api/problem-message";

// Stands in for i18next with every key translated: what matters here is
// which key the refusal was turned into, not the words behind it.
const t = ((key: string) => key) as never;

describe("problemMessage", () => {
  // The network failing is the commonest error a screen sees and the one
  // least likely to arrive as a Problem. It has to come back as a sentence,
  // not as a blown stack: a toast that never renders reads as nothing
  // happening at all.
  it("answers for something that is not a refusal from the server", () => {
    expect(problemMessage(new TypeError("Failed to fetch"), t)).toBe(
      "common.requestFailed",
    );
  });

  it("says what the server refused, in this console's words", () => {
    const error = new ApiError(409, {
      type: "fuseone:conflict",
      title: "Conflict",
      status: 409,
      detail: "triage: 1 correction(s) no longer hold — caso-estorno.",
    });
    expect(problemMessage(error, t)).toBe("problem.conflict");
  });

  it("keeps a busy upstream distinct from a refusal", () => {
    const error = new ApiError(503, {
      type: "fuseone:upstream-busy",
      title: "Upstream busy",
      status: 503,
      detail: "model_provider_overloaded",
    });
    expect(problemMessage(error, t)).toBe("problem.upstream-busy");
  });
});
