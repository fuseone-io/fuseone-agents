import { describe, expect, it } from "vitest";
import { detailOf } from "@/features/runs/step-story";

/*
What a finished run says in the trail.

The exception is the author's words and the trail says the agent asserted
them, never that anything checked: the condition is a sentence about the
world, and the platform has no way to evaluate one.
*/

describe("a run that stopped where the author said it would", () => {
  it("quotes the exception, and keeps what the agent said about it", () => {
    // The author's words, and the trail says the agent asserted them rather
    // than that anything checked: the condition is a sentence about the
    // world, and the platform has no way to evaluate one.
    const line = detailOf({
      seq: 9,
      kind: "run_finished",
      at: "2026-08-14T12:00:00Z",
      payload: {
        outcome: "Procurei pelos dois e-mails.",
        stopped_by: "não encontrar o cliente",
      },
    } as never);

    expect(line.key).toBe("runs.stoppedByException");
    expect(line.values).toMatchObject({ what: "não encontrar o cliente" });
  });

  it("says only the outcome when nothing was asserted", () => {
    const line = detailOf({
      seq: 9,
      kind: "run_finished",
      at: "2026-08-14T12:00:00Z",
      payload: { outcome: "Respondi e encerrei." },
    } as never);

    expect(line.key).toBe("Respondi e encerrei.");
  });
});
