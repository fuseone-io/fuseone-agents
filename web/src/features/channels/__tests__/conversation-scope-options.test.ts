import { describe, expect, it } from "vitest";
import { offeredScopes } from "@/features/channels/conversation-scope-options";
import type { Me } from "@/features/session/api";

// NT-019 phase 4: a conversation is offered only the scopes its connection
// carries and the caller configures; the room for the whole installation only
// on the installation's connection, to whoever governs the installation.

const areas = [
  { company: "acme", area: "cx" },
  { company: "acme", area: "finance" },
  { company: "globex", area: "ops" },
];
const areaAdmin = { can: ["provider:write"], platform: [], grants: [], configure: [{ company: "acme", area: "cx" }] } as unknown as Me;
const governor = { can: ["provider:write", "company:write"], platform: ["provider:write"], grants: [{ company: "*", area: "", role: "admin" }], configure: [] } as unknown as Me;
const companyWriter = { can: ["provider:write", "company:write"], platform: ["provider:write"], grants: [{ company: "acme", area: "", role: "admin" }], configure: [] } as unknown as Me;

describe("the scopes a conversation is offered", () => {
  it("are the caller's own, inside an area's connection", () => {
    const got = offeredScopes(areas, { scopeKind: "area", company: "acme", area: "cx" }, areaAdmin);
    expect(got).toEqual({ installation: false, scopes: [{ company: "acme", area: "cx" }] });
  });

  // On the platform's connection, which carries every scope, what bounds an
  // area's administrator is where they configure.
  it("are the caller's own on the installation's connection", () => {
    const got = offeredScopes(areas, { scopeKind: "installation" }, areaAdmin);
    expect(got.scopes).toEqual([{ company: "acme", area: "cx" }]);
  });

  it("are a company's areas on the company's connection, for the platform", () => {
    const got = offeredScopes(areas, { scopeKind: "company", company: "acme" }, governor);
    expect(got.scopes).toEqual([{ company: "acme", area: "cx" }, { company: "acme", area: "finance" }]);
    expect(got.installation).toBe(false);
  });

  it("offer the whole installation only to whoever governs it, on the installation's connection", () => {
    expect(offeredScopes(areas, { scopeKind: "installation" }, governor).installation).toBe(true);
    expect(offeredScopes(areas, { scopeKind: "installation" }, companyWriter).installation).toBe(false);
    expect(offeredScopes(areas, { scopeKind: "installation" }, areaAdmin).installation).toBe(false);
  });
});
