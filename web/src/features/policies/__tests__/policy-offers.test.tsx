import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ConditionBuilder } from "@/features/policies/condition-builder";
import { OwnerField } from "@/features/policies/owner-field";
import { ReachSection } from "@/features/policies/reach-section";
import { reachNamesNobody, BLANK } from "@/features/policies/policy-form";
import { setLocale } from "@/i18n";

const lists = vi.hoisted(() => ({
  agents: {
    data: {
      items: [{ agentId: "security-sentinel", name: "Security Sentinel" }],
    },
  },
  scopes: { data: { items: [{ company: "cora", area: "platform" }] } },
  people: { data: { items: [{ id: "usr_ana", display: "Ana" }] }, error: null },
  me: { data: { id: "usr_kleber", display: "Kleber Rocha" } },
}));

vi.mock("@/features/agents/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/features/agents/api")>()),
  useAgents: () => lists.agents,
}));
vi.mock("@/features/scope/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/features/scope/api")>()),
  useScopes: () => lists.scopes,
}));
vi.mock("@/features/admin/people-api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/features/admin/people-api")>()),
  usePeople: () => lists.people,
}));
vi.mock("@/features/session/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/features/session/api")>()),
  useMe: () => lists.me,
}));

beforeEach(() => {
  setLocale("en-US");
  Element.prototype.hasPointerCapture ??= () => false;
  Element.prototype.scrollIntoView ??= () => {};
});

describe("condition values come from what the platform knows", () => {
  // The suggestions help; typing stays just as valid. A policy may name an
  // agent that is not deployed yet, so the field is an input with suggestions
  // beside it, never a closed list.
  it("offers the agents and still accepts free text", async () => {
    const onChange = vi.fn();
    render(
      <ConditionBuilder
        conditions={[{ field: "agent.id", op: "eq", value: "" }]}
        onChange={onChange}
      />,
    );

    await userEvent.click(
      screen.getByRole("combobox", { name: "Suggest known values" }),
    );
    await userEvent.click(await screen.findByText("security-sentinel"));
    expect(onChange).toHaveBeenCalledWith([
      { field: "agent.id", op: "eq", value: "security-sentinel" },
    ]);

    // The input is controlled by the parent and this harness never
    // re-renders, so each keystroke reports from the same base value. One
    // keystroke is therefore the sharp assertion: the typed character must
    // arrive in onChange, or the field has stopped accepting free text.
    onChange.mockClear();
    await userEvent.type(screen.getByLabelText("Condition 1 value"), "x");
    expect(onChange).toHaveBeenCalledWith([
      { field: "agent.id", op: "eq", value: "x" },
    ]);
  });

  // Under `in` the value is a set: picking appends, picking again removes.
  it("appends to the set under the in operator", async () => {
    let conditions = [
      { field: "agent.id", op: "in" as const, value: "triage" },
    ];
    const onChange = vi.fn((next) => (conditions = next));
    render(<ConditionBuilder conditions={conditions} onChange={onChange} />);
    await userEvent.click(
      screen.getByRole("combobox", { name: "Suggest known values" }),
    );
    await userEvent.click(await screen.findByText("security-sentinel"));
    expect(onChange).toHaveBeenCalledWith([
      { field: "agent.id", op: "in", value: "triage,security-sentinel" },
    ]);
  });

  // Changing the subject clears the value: an agent id left behind in a
  // tool.effect clause is a condition that can never hold.
  it("clears the value when the field changes", async () => {
    const onChange = vi.fn();
    render(
      <ConditionBuilder
        conditions={[{ field: "agent.id", op: "eq", value: "triage" }]}
        onChange={onChange}
      />,
    );
    await userEvent.click(screen.getByRole("combobox", { name: "Field" }));
    await userEvent.click(await screen.findByText("the tool's effect"));
    expect(onChange).toHaveBeenCalledWith([
      { field: "tool.effect", op: "eq", value: "" },
    ]);
  });
});

describe("the owner field", () => {
  it("starts owned by whoever is creating", async () => {
    const onChange = vi.fn();
    render(<OwnerField creating value="" onChange={onChange} />);
    expect(onChange).toHaveBeenCalledWith("Kleber Rocha");
  });

  it("does not overwrite an owner somebody typed", () => {
    const onChange = vi.fn();
    render(
      <OwnerField creating value="Time de Plataforma" onChange={onChange} />,
    );
    expect(onChange).not.toHaveBeenCalled();
  });

  // The page is visible to roles the directory refuses. A refusal leaves a
  // plain input and no error on screen.
  it("stays a plain input when the directory refuses", () => {
    lists.people = { data: undefined, error: new Error("403") } as never;
    const onChange = vi.fn();
    render(<OwnerField creating={false} value="x" onChange={onChange} />);
    expect(screen.getByLabelText("Owner")).toBeInTheDocument();
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    lists.people = {
      data: { items: [{ id: "usr_ana", display: "Ana" }] },
      error: null,
    } as never;
  });
});

describe("the reach section", () => {
  it("a reach that names nobody blocks the save", () => {
    expect(reachNamesNobody({ ...BLANK, reach: "agents", agents: [] })).toBe(
      true,
    );
    expect(
      reachNamesNobody({ ...BLANK, reach: "agents", agents: ["triage"] }),
    ).toBe(false);
    expect(reachNamesNobody({ ...BLANK, reach: "installation" })).toBe(false);
  });

  it("chooses agents from the list and keeps them as removable chips", async () => {
    const patch = vi.fn();
    render(
      <ReachSection
        draft={{ ...BLANK, reach: "agents", agents: ["already-chosen"] }}
        patch={patch}
      />,
    );
    expect(screen.getByText("already-chosen")).toBeInTheDocument();
    await userEvent.click(
      screen.getByRole("combobox", { name: "Choose agents" }),
    );
    await userEvent.click(await screen.findByText("security-sentinel"));
    expect(patch).toHaveBeenCalledWith({
      agents: ["already-chosen", "security-sentinel"],
    });
  });
});
