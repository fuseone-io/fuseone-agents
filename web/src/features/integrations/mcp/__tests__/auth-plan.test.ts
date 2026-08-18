import { describe, expect, it } from "vitest";
import { remoteAuthPlan } from "@/features/integrations/mcp/auth-plan";

describe("remote MCP auth planning", () => {
  it("treats one named header as an editable remote credential", () => {
    const plan = remoteAuthPlan(
      [{ type: "headers", principal: "service", header: "Api-Key" }],
      true,
    );

    expect(plan.secret?.type).toBe("headers");
    expect(plan.header?.header).toBe("Api-Key");
    expect(plan.unsupported).toHaveLength(0);
  });

  it("treats named multi-header auth as editable without pretending it is one secret", () => {
    const plan = remoteAuthPlan(
      [
        {
          type: "headers",
          principal: "service",
          label: "API and app keys",
          headers: ["DD_API_KEY", "DD_APPLICATION_KEY"],
        },
      ],
      true,
    );

    expect(plan.secret).toBeNull();
    expect(plan.multiHeaders?.headers).toEqual(["DD_API_KEY", "DD_APPLICATION_KEY"]);
    expect(plan.unsupported).toHaveLength(0);
  });

  it("keeps unshaped header auth unsupported until the recipe names the headers", () => {
    const plan = remoteAuthPlan(
      [{ type: "headers", principal: "service", label: "API and app keys" }],
      true,
    );

    expect(plan.secret).toBeNull();
    expect(plan.unsupported).toHaveLength(1);
  });

  it("uses the first editable secret the recipe declares", () => {
    const plan = remoteAuthPlan(
      [
        { type: "basic", principal: "user", header: "Authorization", prefix: "Basic" },
        { type: "bearer", principal: "service" },
      ],
      true,
    );

    expect(plan.secret?.type).toBe("basic");
  });
});
