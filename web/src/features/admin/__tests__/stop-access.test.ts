import { describe, expect, it } from "vitest";
import { EVERYTHING, stopTargetsFor } from "@/features/admin/stop-access";
import type { RegisteredScope } from "@/features/scope/api";
import type { Me } from "@/features/session/api";

const scope = (
  company: string,
  area: string,
  label = area,
): RegisteredScope => ({
  company,
  area,
  label,
});

const me = (role: string, company: string, area: string): Me => ({
  id: "usr_ana",
  display: "Ana",
  kind: "human",
  can: [],
  grants: [{ role, company, area }],
});

describe("stop targets", () => {
  it("does not offer the installation switch to somebody granted only in an area", () => {
    const targets = stopTargetsFor(me("author", "acme", "devops"), [
      scope("acme", "devops", "DevOps"),
      scope("acme", "finance", "Finance"),
    ]);

    expect(targets.map((target) => target.value)).toEqual(["acme/devops"]);
  });

  it("lets an installation grant stop the installation and every area", () => {
    const targets = stopTargetsFor(me("curator", "*", ""), [
      scope("acme", "devops", "DevOps"),
    ]);

    expect(targets.map((target) => target.value)).toEqual([
      EVERYTHING,
      "acme/devops",
    ]);
  });

  it("preserves the open-installation mode where there is no identity to filter by", () => {
    const targets = stopTargetsFor(null, [scope("acme", "devops", "DevOps")]);

    expect(targets.map((target) => target.value)).toEqual([
      EVERYTHING,
      "acme/devops",
    ]);
  });
});
