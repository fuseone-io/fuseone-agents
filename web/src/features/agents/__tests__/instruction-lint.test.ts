import { describe, expect, it } from "vitest";
import { findings } from "@/features/agents/instruction-lint";
import type { Policy, Tool } from "@/lib/api/client";

/*
Where the prose promises what the platform already refuses.

The sentence will never come true, and the person who finds that out is
whoever reads the run afterwards wondering why it stopped. It stays a finding
rather than a refusal: the text may be explaining the rule to the next reader,
which is a good reason to keep it and the author's to give.
*/

const CATALOGUE: Tool[] = [
  { toolId: "crm.lookup", server: "crm", effect: "read", untrusted: true },
  { toolId: "erp.refund", server: "erp", effect: "financial", untrusted: true },
];

/*
A policy that refuses a tool the built-in ladder would have allowed.

Deliberately a read: the ladder already blocks anything financial, so a policy
saying so again is not what decided, and citing it would send somebody to
change a rule that changes nothing.
*/
const DENIES = [
  {
    code: "POL-114",
    name: "Sem consulta ao CRM nesta área",
    resource: "crm.lookup",
    effect: "deny",
    // Enforcing rather than observing: a rule somebody is only watching
    // does not refuse anything, so the prose is not promising the impossible.
    mode: "enforce",
    enabled: true,
  },
] as unknown as Policy[];

describe("what an instruction promises and the policy refuses", () => {
  it("names the block, the tool and the rule", () => {
    const found = findings(
      [
        { kind: "objective", text: "Compare os dois lados." },
        { kind: "howToAct", text: "Se precisar, use crm.lookup." },
      ],
      CATALOGUE,
      DENIES,
    );

    // The block, because that is where it has to be answered — a banner at
    // the top of the card leaves somebody hunting for the sentence. And the
    // rule by name, because "blocked by policy" tells an author nothing about
    // what to change and cannot tell two rules apart.
    expect(found).toEqual([{ at: 1, tool: "crm.lookup", because: "POL-114" }]);
  });

  it("says nothing about a tool nothing refuses", () => {
    expect(
      findings(
        [{ kind: "objective", text: "Use crm.lookup." }],
        CATALOGUE,
        [],
      ),
    ).toEqual([]);
  });

  it("still fires when it is the ladder refusing, and names no rule", () => {
    // With no policy at all the built-in ladder already refuses a financial
    // call, so the sentence is a promise that cannot be kept either way. What
    // changes is what can be cited: there is no code to point at, and saying
    // "blocked by policy" when no policy exists would send somebody looking
    // for one.
    expect(
      findings([{ kind: "objective", text: "Use erp.refund." }], CATALOGUE, []),
    ).toEqual([{ at: 0, tool: "erp.refund", because: undefined }]);
  });
});
