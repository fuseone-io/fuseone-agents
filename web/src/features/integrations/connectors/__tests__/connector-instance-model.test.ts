import { describe, expect, it } from "vitest";
import {
  connectorInstanceDefaults,
  connectorInstancePayload,
  connectorInstanceSchema,
} from "@/features/integrations/connectors/connector-instance-model";
import type { ConnectorInstance } from "@/features/integrations/api";

const stored: ConnectorInstance = {
  name: "prod",
  connector: "vault",
  enabled: true,
  scopeKind: "area",
  company: "acme",
  area: "platform",
  hasToken: true,
  vault: {
    address: "https://vault.example.com",
    mount: "secret",
    namespace: "admin",
    allowedPathPrefixes: ["certificates"],
  },
};

describe("connector instance form model", () => {
  it("builds a Vault instance with an area scope and a new sealed token", () => {
    const values = connectorInstanceDefaults(null, "acme", "platform");
    values.name = "prod";
    values.address = "https://vault.example.com";
    values.allowedPathPrefixes = "certificates\npki/issued";
    values.token = "vault-token";

    expect(connectorInstancePayload(values, false)).toEqual({
      name: "prod",
      body: {
        connector: "vault",
        enabled: true,
        scopeKind: "area",
        company: "acme",
        area: "platform",
        token: "vault-token",
        vault: {
          address: "https://vault.example.com",
          mount: "secret",
          allowedPathPrefixes: ["certificates", "pki/issued"],
        },
      },
    });
  });

  it("omits token and clearToken when editing leaves the stored token alone", () => {
    const values = connectorInstanceDefaults(stored, "ignored", "ignored");
    values.address = "https://vault.internal";

    const payload = connectorInstancePayload(values, true);
    expect(payload?.body.token).toBeUndefined();
    expect(payload?.body.clearToken).toBeUndefined();
    expect(payload?.body.vault?.address).toBe("https://vault.internal");
  });

  it("does not build an enabled instance that removes its only token", () => {
    const values = connectorInstanceDefaults(stored, "ignored", "ignored");
    values.clearToken = true;

    expect(connectorInstancePayload(values, true)).toBeNull();
  });

  it("does not build an enabled instance without a stored or new token", () => {
    const values = connectorInstanceDefaults(null, "acme", "platform");
    values.name = "prod";
    values.address = "https://vault.example.com";
    values.allowedPathPrefixes = "certificates";

    expect(connectorInstancePayload(values, false)).toBeNull();
  });

  it("does not build an instance without an allowed Vault path prefix", () => {
    const values = connectorInstanceDefaults(null, "acme", "platform");
    values.name = "prod";
    values.address = "https://vault.example.com";
    values.allowedPathPrefixes = " , \n ";
    values.token = "vault-token";

    expect(connectorInstancePayload(values, false)).toBeNull();
  });

  it("does not leak company or area fields into an installation instance", () => {
    const values = connectorInstanceDefaults(null, "acme", "platform");
    values.name = "global";
    values.scopeKind = "installation";
    values.address = "https://vault.example.com";
    values.allowedPathPrefixes = "shared";
    values.token = "vault-token";

    const payload = connectorInstancePayload(values, false);
    expect(payload?.body.company).toBeUndefined();
    expect(payload?.body.area).toBeUndefined();
  });

  it("refuses blank path prefixes at the field that owns them", () => {
    const values = connectorInstanceDefaults(null, "acme", "platform");
    values.name = "prod";
    values.address = "https://vault.example.com";
    values.allowedPathPrefixes = " , \n ";
    values.token = "vault-token";

    const result = connectorInstanceSchema.safeParse(values);
    expect(result.success).toBe(false);
    if (result.success) throw new Error("blank prefixes were accepted");
    expect(result.error.issues).toContainEqual(
      expect.objectContaining({
        path: ["allowedPathPrefixes"],
        message: "connectors.pathPrefixesRequired",
      }),
    );
  });
});
