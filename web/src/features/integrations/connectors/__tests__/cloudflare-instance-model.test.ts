import { describe, expect, it } from "vitest";
import {
  cloudflareInstanceDefaults,
  cloudflareInstancePayload,
  cloudflareInstanceSchema,
} from "@/features/integrations/connectors/cloudflare-instance-model";
import type { ConnectorInstance } from "@/features/integrations/api";

const stored: ConnectorInstance = {
  name: "edge",
  connector: "cloudflare",
  enabled: true,
  scopeKind: "area",
  company: "acme",
  area: "platform",
  hasToken: true,
  cloudflare: {
    accountId: "acct1234",
    listId: "list5678",
    protectedRanges: ["203.0.113.0/24", "2001:db8::/32"],
    maxBlocksPerDay: 50,
  },
};

function values() {
  return cloudflareInstanceDefaults(stored, "acme", "platform");
}

describe("the cloudflare instance model", () => {
  it("round-trips the stored configuration and never prefills the token", () => {
    const defaults = values();
    expect(defaults.accountId).toBe("acct1234");
    expect(defaults.protectedRanges).toBe("203.0.113.0/24\n2001:db8::/32");
    expect(defaults.maxBlocksPerDay).toBe(50);
    expect(defaults.token).toBe("");

    const payload = cloudflareInstancePayload(defaults, true);
    expect(payload?.body.cloudflare).toEqual({
      accountId: "acct1234",
      listId: "list5678",
      protectedRanges: ["203.0.113.0/24", "2001:db8::/32"],
      maxBlocksPerDay: 50,
    });
    // Nothing typed, nothing cleared: the stored token stays stored.
    expect(payload?.body.token).toBeUndefined();
    expect(payload?.body.clearToken).toBeUndefined();
  });

  it("refuses to enable without a token anywhere", () => {
    const payload = cloudflareInstancePayload(values(), false);
    expect(payload).toBeNull();
  });

  it("sends the token only when typed, and clearToken only alone", () => {
    const typed = { ...values(), token: "cf-token" };
    expect(cloudflareInstancePayload(typed, false)?.body.token).toBe(
      "cf-token",
    );

    const cleared = { ...values(), enabled: false, clearToken: true };
    expect(cloudflareInstancePayload(cleared, true)?.body.clearToken).toBe(
      true,
    );
  });

  it("rejects a protected range that is not a CIDR", () => {
    const bare = { ...values(), protectedRanges: "203.0.113.9" };
    const result = cloudflareInstanceSchema.safeParse(bare);
    expect(result.success).toBe(false);
  });

  it("rejects an http endpoint", () => {
    const result = cloudflareInstanceSchema.safeParse({
      ...values(),
      baseUrl: "http://api.cloudflare.com",
    });
    expect(result.success).toBe(false);
  });
});
