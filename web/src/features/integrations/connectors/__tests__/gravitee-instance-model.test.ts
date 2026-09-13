import { describe, expect, it } from "vitest";
import type {
  ConnectorInstance,
  ConnectorInstanceDetail,
} from "@/features/integrations/api";
import {
  graviteeBindingIssue,
  graviteeInstanceDefaults,
  graviteeInstancePayload,
  graviteeInstanceSchema,
} from "@/features/integrations/connectors/gravitee-instance-model";

const detail: ConnectorInstanceDetail = {
  name: "apim",
  connector: "gravitee",
  enabled: true,
  scopeKind: "area",
  company: "acme",
  area: "platform",
  hasToken: false,
  gravitee: {
    address: "https://apim.internal/management/v2",
    organization: "org-prod",
    environment: "env-prod",
    allowedReferences: [{ type: "API", id: "checkout-api" }],
    minTTLSeconds: 86_400,
    maxTTLSeconds: 7_776_000,
    allowNoExpiry: false,
    credentialSource: {
      kind: "vault_kv_secret",
      vaultInstance: "secrets",
      path: "integrations/gravitee/prod",
      field: "access_token",
    },
  },
};

function vault(overrides: Partial<ConnectorInstance> = {}): ConnectorInstance {
  return {
    name: "secrets",
    connector: "vault",
    enabled: true,
    scopeKind: "company",
    company: "acme",
    hasToken: true,
    vault: {
      address: "https://vault.internal",
      mount: "secret",
      allowedPathPrefixes: ["integrations/gravitee"],
    },
    ...overrides,
  };
}

describe("Gravitee connector instance form model", () => {
  it("round-trips the complete authored boundary without inventing a token", () => {
    const values = graviteeInstanceDefaults(detail, "ignored", "ignored");

    expect(graviteeInstancePayload(values)).toEqual({
      name: "apim",
      body: {
        connector: "gravitee",
        enabled: true,
        scopeKind: "area",
        company: "acme",
        area: "platform",
        gravitee: detail.gravitee,
      },
    });
  });

  it("creates the exact API contract from the three fixed boundaries", () => {
    const values = {
      ...graviteeInstanceDefaults(null, "acme", "platform"),
      name: "apim",
      address: "https://apim.internal/management/v2",
      organization: "org-prod",
      environment: "env-prod",
      apiReference: "checkout-api",
      vaultInstance: "secrets",
      credentialPath: "integrations/gravitee/prod",
    };

    expect(graviteeInstancePayload(values)).toEqual({
      name: "apim",
      body: {
        connector: "gravitee",
        enabled: true,
        scopeKind: "area",
        company: "acme",
        area: "platform",
        gravitee: detail.gravitee,
      },
    });
  });

  it("does not turn the global scope filter into a stored wildcard", () => {
    expect(graviteeInstanceDefaults(null, "*", "*")).toMatchObject({
      scopeKind: "area",
      company: "",
      area: "",
    });
  });

  it.each([
    ["plain HTTP", { address: "http://apim.internal" }],
    ["URL credential", { address: "https://user:secret@apim.internal" }],
    ["URL query", { address: "https://apim.internal?token=secret" }],
    ["opaque HTTPS URL", { address: "https:apim.internal" }],
    ["relative URL segment", { address: "https://apim.internal/a/../admin" }],
    ["metadata address", { address: "https://169.254.169.254" }],
    ["IPv4-mapped metadata address", { address: "https://[::ffff:169.254.1.1]" }],
    ["invalid organization", { organization: "../org" }],
    ["invalid API", { apiReference: "another/api" }],
    ["inverted TTL", { minTTLSeconds: 20, maxTTLSeconds: 10 }],
    ["TTL over one year", { maxTTLSeconds: 31_536_001 }],
    ["credential traversal", { credentialPath: "integrations/../admin" }],
    ["credential field as path", { credentialField: "data/token" }],
  ])("refuses %s before it reaches the server", (_name, change) => {
    const values = { ...graviteeInstanceDefaults(detail, "ignored", "ignored"), ...change };
    expect(graviteeInstanceSchema.safeParse(values).success).toBe(false);
  });

  it("requires the chosen Vault to be unique, usable and to own the credential path", () => {
    const target = { scopeKind: "area" as const, company: "acme", area: "platform" };
    expect(graviteeBindingIssue([vault()], target, "secrets", "integrations/gravitee/prod"))
      .toBeNull();
    expect(graviteeBindingIssue([vault()], target, "secrets", "other/service"))
      .toBe("connectors.graviteeVaultPathOutside");
    expect(graviteeBindingIssue([vault({ hasToken: false })], target, "secrets", "integrations/gravitee/prod"))
      .toBe("connectors.graviteeVaultUnavailable");
    expect(graviteeBindingIssue([
      vault(),
      vault({ scopeKind: "area", area: "platform" }),
    ], target, "secrets", "integrations/gravitee/prod"))
      .toBe("connectors.graviteeVaultAmbiguous");
  });

  it("does not let an area Vault supply a wider company instance", () => {
    const target = { scopeKind: "company" as const, company: "acme", area: "" };
    expect(graviteeBindingIssue([
      vault({ scopeKind: "area", area: "platform" }),
    ], target, "secrets", "integrations/gravitee/prod"))
      .toBe("connectors.graviteeVaultUnavailable");
  });
});
