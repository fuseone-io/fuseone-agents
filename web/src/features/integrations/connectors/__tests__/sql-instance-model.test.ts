import { describe, expect, it } from "vitest";
import type {
  ConnectorInstance,
  ConnectorInstanceDetail,
} from "@/features/integrations/api";
import {
  sqlInstanceDefaults,
  sqlInstancePayload,
  sqlInstanceSchema,
  vaultChoices,
} from "@/features/integrations/connectors/sql-instance-model";

const vault = (
  name: string,
  scopeKind: ConnectorInstance["scopeKind"],
  company?: string,
  area?: string,
): ConnectorInstance => ({
  name,
  connector: "vault",
  enabled: true,
  scopeKind,
  company,
  area,
  hasToken: true,
  vault: {
    address: "https://vault.example.com",
    mount: "secret",
    allowedPathPrefixes: ["database/creds"],
  },
});

const detail: ConnectorInstanceDetail = {
  name: "app-x",
  connector: "sql",
  enabled: true,
  scopeKind: "area",
  company: "acme",
  area: "platform",
  hasToken: false,
  sql: {
    driver: "postgres",
    host: "db.internal",
    port: 5432,
    database: "appx",
    credentialSource: {
      kind: "vault_database_role",
      vaultInstance: "prod",
      mount: "database",
      role: "app-x-readonly",
    },
    templates: [{
      id: "orders_by_customer",
      sql: "select id from orders where customer_id = $1",
      parameters: [{ name: "customer_id", type: "text" }],
      timeoutSeconds: 10,
      maxRows: 200,
      maxBytes: 65_536,
    }],
  },
};

describe("SQL connector instance form model", () => {
  it("round-trips the complete authored SQL contract", () => {
    const values = sqlInstanceDefaults(detail, "ignored", "ignored");

    expect(sqlInstancePayload(values)).toEqual({
      name: "app-x",
      body: {
        connector: "sql",
        enabled: true,
        scopeKind: "area",
        company: "acme",
        area: "platform",
        sql: detail.sql,
      },
    });
  });

  it("refuses duplicate names and placeholders that do not match the parameters", () => {
    const values = sqlInstanceDefaults(detail, "ignored", "ignored");
    values.templates.push({ ...values.templates[0]!, parameters: [] });

    const result = sqlInstanceSchema.safeParse(values);
    expect(result.success).toBe(false);
    if (result.success) throw new Error("an ambiguous SQL contract was accepted");
    expect(result.error.issues.map((issue) => issue.message)).toEqual(
      expect.arrayContaining([
        "connectors.sqlTemplateDuplicate",
        "connectors.sqlPlaceholdersMismatch",
      ]),
    );
  });

  it("requires an enabled instance to register a template", () => {
    const values = sqlInstanceDefaults(detail, "ignored", "ignored");
    values.templates = [];

    expect(sqlInstanceSchema.safeParse(values).success).toBe(false);
    values.enabled = false;
    expect(sqlInstanceSchema.safeParse(values).success).toBe(true);
  });

  it("accepts only a bare hostname or IP address, never a DSN", () => {
    const values = sqlInstanceDefaults(detail, "ignored", "ignored");
    values.host = "postgres://reader:secret@db.internal/appx";
    expect(sqlInstanceSchema.safeParse(values).success).toBe(false);

    values.host = "2001:db8::1";
    expect(sqlInstanceSchema.safeParse(values).success).toBe(true);
  });

  it("offers only unambiguous HTTPS Vaults that contain the SQL scope", () => {
    const choices = vaultChoices([
      vault("global", "installation"),
      vault("prod", "company", "acme"),
      vault("prod", "area", "acme", "platform"),
      vault("mixed", "area", "acme", "platform"),
      { ...vault("mixed", "company", "acme"), enabled: false },
      vault("other", "area", "acme", "payments"),
      { ...vault("plain", "area", "acme", "platform"), vault: {
        address: "http://vault.internal", mount: "secret", allowedPathPrefixes: ["db"],
      } },
    ], { scopeKind: "area", company: "acme", area: "platform" });

    expect(choices).toEqual([
      { name: "global", label: "global · installation", ambiguous: false },
      { name: "mixed", label: "mixed", ambiguous: true },
      { name: "prod", label: "prod", ambiguous: true },
    ]);
  });
});
