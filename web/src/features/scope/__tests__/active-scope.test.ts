import { beforeEach, describe, expect, it } from "vitest";
import { scopeParamsOf, useActiveScope } from "@/features/scope/active-scope";
import type { MeGrant } from "@/features/session/api";

const grant = (company: string, area: string): MeGrant => ({
  company,
  area,
  role: "author",
});

describe("the active scope", () => {
  beforeEach(() => useActiveScope.setState({ company: "", area: "" }));

  it("sends nothing while no context is chosen, so the API answers with everything the caller reaches", () => {
    expect(scopeParamsOf(useActiveScope.getState())).toEqual({});
  });

  it("treats a stored installation wildcard as everything, not as a company filter", () => {
    useActiveScope.getState().choose({ company: "*", area: "" });
    expect(scopeParamsOf(useActiveScope.getState())).toEqual({});
  });

  it("sends the company alone when the whole company is chosen", () => {
    useActiveScope.getState().choose({ company: "acme", area: "" });
    expect(scopeParamsOf(useActiveScope.getState())).toEqual({
      company: "acme",
    });
  });

  it("sends both when an area is chosen", () => {
    useActiveScope.getState().choose({ company: "acme", area: "cx" });
    expect(scopeParamsOf(useActiveScope.getState())).toEqual({
      company: "acme",
      area: "cx",
    });
  });

  it("drops a stored scope the caller no longer reaches, rather than filtering every screen to nothing", () => {
    useActiveScope.getState().choose({ company: "antiga", area: "cx" });
    useActiveScope.getState().reconcile([grant("acme", "")]);
    expect(useActiveScope.getState()).toMatchObject({ company: "", area: "" });
  });

  it("drops a stored wildcard scope after the installation grant moved to the everything choice", () => {
    useActiveScope.getState().choose({ company: "*", area: "" });
    useActiveScope.getState().reconcile([grant("*", "")]);
    expect(useActiveScope.getState()).toMatchObject({ company: "", area: "" });
  });

  it("keeps a real company chosen by an installation administrator", () => {
    useActiveScope.getState().choose({ company: "acme", area: "cx" });
    useActiveScope.getState().reconcile([grant("*", "")]);
    expect(useActiveScope.getState()).toMatchObject({
      company: "acme",
      area: "cx",
    });
  });

  it("keeps an area chosen inside a company the caller holds wholesale", () => {
    useActiveScope.getState().choose({ company: "acme", area: "cx" });
    useActiveScope.getState().reconcile([grant("acme", "")]);
    expect(useActiveScope.getState()).toMatchObject({
      company: "acme",
      area: "cx",
    });
  });
});
