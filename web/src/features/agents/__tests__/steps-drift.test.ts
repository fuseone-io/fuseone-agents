import { describe, expect, it } from "vitest";
import { contradictions, undescribed } from "@/features/agents/steps-drift";
import type { Tool } from "@/lib/api/client";

/*
The drawing and the words are authored separately, so they can disagree.

Which is fine in one direction and not in the other: prose may say more than
the permissions do, and permissions saying more than the prose means an agent
is allowed something nobody wrote down.
*/

describe("what the drawing says and the instructions do not", () => {
  it("names a tool a stage reaches that the text never mentions", () => {
    expect(
      undescribed(
        [{ name: "Pagar", reaches: ["erp.transfer"] }],
        "Responda o cliente e encerre o chamado.",
      ),
    ).toEqual(["erp.transfer"]);
  });

  it("accepts the tool named without its server", () => {
    // Demanding the qualified identifier would fire on every agent whose
    // author writes like a person.
    expect(
      undescribed(
        [{ name: "Encontrar", reaches: ["crm.lookup"] }],
        "Use o lookup para achar o cliente pelo e-mail.",
      ),
    ).toEqual([]);
  });

  it("says nothing about prose that describes more than was drawn", () => {
    // The safe direction: instructions may say more than the permissions do.
    expect(
      undescribed([{ name: "Pensar" }], "Consulte o CRM, responda e encerre."),
    ).toEqual([]);
  });
});

/*
What the instructions forbid and the drawing still does.

This is the other unsafe direction: a Never block is not policy, but a screen
that lets a step keep the opposite instruction hidden in another tab is how a
run later appears to ignore its author.
*/
describe("what Never forbids and the stages still carry", () => {
  it("names the stage that still stops on the forbidden subject", () => {
    expect(
      contradictions(
        [
          {
            name: "Diagnosticar",
            stopsWhen: "Se o alerta possuir runbook, consultar via Outline.",
          },
        ],
        "Nunca\nNão tente ler o runbook; ignore essa parte do runbook.",
        [],
      ),
    ).toEqual([{ at: 0, why: "forbiddenStop", term: "runbook" }]);
  });

  it("does not treat generic action verbs in Never as forbidden subjects", () => {
    expect(
      contradictions(
        [
          {
            name: "Identificar",
            stopsWhen:
              "Se faltar namespace ou horário para consultar com segurança, responda pedindo os dados.",
          },
        ],
        "Nunca\nNunca diga vou analisar, vou consultar, vou verificar ou vou prosseguir.",
        [],
      ),
    ).toEqual([]);
  });

  it("names a tool the stage can still reach after Never names it", () => {
    const catalogue: Tool[] = [
      {
        toolId: "outline.list_documents",
        server: "outline",
        effect: "read",
        untrusted: true,
      },
    ];

    expect(
      contradictions(
        [{ name: "Buscar", reaches: ["outline.list_documents"] }],
        "Never\nDo not use Outline for this run.",
        catalogue,
      ),
    ).toEqual([
      {
        at: 0,
        why: "forbiddenReach",
        term: "outline.list_documents",
        tool: "outline.list_documents",
      },
    ]);
  });

  it("uses the pack when the agent has no stages", () => {
    const catalogue: Tool[] = [
      {
        toolId: "outline.list_documents",
        server: "outline",
        effect: "read",
        untrusted: true,
      },
    ];

    expect(
      contradictions(
        [],
        "Never\nDo not use Outline for this run.",
        catalogue,
        ["outline.list_documents"],
      ),
    ).toEqual([
      {
        why: "forbiddenReach",
        term: "outline.list_documents",
        tool: "outline.list_documents",
      },
    ]);
  });
});
