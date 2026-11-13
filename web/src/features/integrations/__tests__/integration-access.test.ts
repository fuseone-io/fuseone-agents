import { describe, expect, it } from "vitest";
import { integrationAccess } from "@/features/integrations/integration-access";
import type { Me } from "@/features/session/api";

// An area's administrator configures its own connector instances and MCP
// servers, and nothing else under integrations (NT-019).

function me(over: Partial<Me>): Me {
  return { id: "usr_ana", display: "Ana", kind: "human", grants: [], can: [], ...over };
}

describe("integration access", () => {
  it("opens the connectors and the MCP servers to an area's administrator, in its own scopes", () => {
    const access = integrationAccess(
      me({ can: ["tool:read", "provider:write"], platform: [], configure: [{ company: "acme", area: "cx" }] }),
    );

    expect(access).toMatchObject({
      platform: false, platformConfigure: false, connectors: true, createConnector: true, anyScope: false,
      servers: true, createServer: true,
    });
    expect(access.configure).toEqual([{ company: "acme", area: "cx" }]);
  });

  it("leaves every scope free to the platform's administration", () => {
    const access = integrationAccess(
      me({ can: ["tool:read", "provider:write"], platform: ["tool:read", "provider:write"] }),
    );

    expect(access).toMatchObject({ platform: true, platformConfigure: true, anyScope: true });
  });

  it("opens nothing to an author of one area", () => {
    const access = integrationAccess(me({ can: ["tool:read"], platform: [] }));

    expect(access).toMatchObject({
      platform: false, connectors: false, createConnector: false, servers: false, createServer: false,
    });
  });
});
