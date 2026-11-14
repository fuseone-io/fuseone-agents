import { describe, expect, it } from "vitest";
import type { MCPServer } from "@/features/integrations/api";
import { catalogueTargets } from "@/features/integrations/mcp/use-catalogue-tools";

// A target is answered only inside the caller's own scopes (readerReach).

const at = (name: string, company: string, area = ""): MCPServer =>
  ({ name, transport: "http", enabled: true, scopeKind: area ? "area" : "company", company, area }) as MCPServer;
const installation = { name: "docs", transport: "http", enabled: true, scopeKind: "installation" } as MCPServer;

describe("the scopes the catalogue asks tools for", () => {
  it("asks an area's administrator for their area alone, which holds what it inherits", () => {
    const servers = [at("crm", "acme", "cx"), at("wiki", "acme"), at("ops-bot", "acme", "ops"), installation];
    expect(catalogueTargets(servers, [{ company: "acme", area: "cx" }])).toEqual(["acme/cx"]);
  });

  it("asks a company's administrator for the company and each of its areas that holds a server", () => {
    const servers = [at("crm", "acme", "cx"), at("wiki", "acme"), at("other", "globex", "cx")];
    expect(catalogueTargets(servers, [{ company: "acme", area: "" }])).toEqual(["acme", "acme/cx"]);
  });
});
