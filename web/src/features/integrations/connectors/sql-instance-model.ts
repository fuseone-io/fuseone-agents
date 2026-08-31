import { z } from "zod";
import type {
  ConnectorInstance,
  ConnectorInstanceDetail,
  ConnectorInstanceInput,
} from "@/features/integrations/api";
import {
  applyConnectorScope,
  connectorInstanceName,
  type ConnectorInstanceSaveInput,
} from "@/features/integrations/connectors/connector-instance-model";

const identifier = /^[a-z][a-z0-9_]{0,63}$/;
const hostname = /^[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$/;
const ipAddress = z.string().ip();
const parameterType = z.enum([
  "text",
  "integer",
  "number",
  "boolean",
  "timestamp",
]);

const sqlParameterSchema = z.object({
  name: z.string().regex(identifier, "connectors.sqlParameterNameInvalid"),
  type: parameterType,
});

const sqlTemplateSchema = z.object({
  id: z.string().regex(identifier, "connectors.sqlTemplateIdInvalid"),
  sql: z
    .string()
    .refine((value) => value.trim() !== "", "connectors.sqlQueryRequired")
    .refine(
      (value) => new TextEncoder().encode(value).length <= 16_384,
      "connectors.sqlQueryTooLong",
    ),
  parameters: z.array(sqlParameterSchema).max(32),
  timeoutSeconds: z.number().int().min(1).max(3600),
  maxRows: z.number().int().min(1).max(10_000),
  maxBytes: z.number().int().min(1024).max(1_048_576),
});

export const sqlInstanceSchema = z
  .object({
    name: z.string().regex(connectorInstanceName, "connectors.instanceNameInvalid"),
    enabled: z.boolean(),
    scopeKind: z.enum(["installation", "company", "area"]),
    company: z.string(),
    area: z.string(),
    driver: z.literal("postgres"),
    host: z
      .string()
      .trim()
      .min(1, "connectors.sqlHostRequired")
      .refine(
        (value) => hostname.test(value) || ipAddress.safeParse(value).success,
        "connectors.sqlHostInvalid",
      ),
    port: z.number().int().min(1).max(65_535),
    database: z.string().trim().min(1, "connectors.sqlDatabaseRequired"),
    vaultInstance: z.string().trim().min(1, "connectors.sqlVaultRequired"),
    credentialMount: z.string().trim().min(1, "connectors.sqlCredentialMountRequired"),
    credentialRole: z.string().trim().min(1, "connectors.sqlCredentialRoleRequired"),
    templates: z.array(sqlTemplateSchema).max(64),
  })
  .superRefine((values, ctx) => {
    requireScope(values, ctx);
    if (values.enabled && values.templates.length === 0) {
      issue(ctx, ["templates"], "connectors.sqlTemplateRequired");
    }
    unique(values.templates.map((template) => template.id)).forEach((id) => {
      const index = values.templates.findIndex((template) => template.id === id);
      issue(ctx, ["templates", index, "id"], "connectors.sqlTemplateDuplicate");
    });
    values.templates.forEach((template, templateIndex) => {
      unique(template.parameters.map((parameter) => parameter.name)).forEach((name) => {
        const index = template.parameters.findIndex((parameter) => parameter.name === name);
        issue(
          ctx,
          ["templates", templateIndex, "parameters", index, "name"],
          "connectors.sqlParameterDuplicate",
        );
      });
      const placeholders = new Set(
        [...template.sql.matchAll(/\$(\d+)/g)].map((match) => Number(match[1])),
      );
      const complete = template.parameters.every((_, index) => placeholders.has(index + 1));
      if (placeholders.size !== template.parameters.length || !complete) {
        issue(
          ctx,
          ["templates", templateIndex, "sql"],
          "connectors.sqlPlaceholdersMismatch",
        );
      }
    });
  });

export type SQLInstanceValues = z.infer<typeof sqlInstanceSchema>;
export type SQLTemplateValues = SQLInstanceValues["templates"][number];

export function sqlInstanceDefaults(
  instance: ConnectorInstanceDetail | null,
  company: string,
  area: string,
): SQLInstanceValues {
  const sql = instance?.sql;
  return {
    name: instance?.name ?? "",
    enabled: instance?.enabled ?? true,
    scopeKind: instance?.scopeKind ?? "area",
    company: instance?.company ?? company,
    area: instance?.area ?? area,
    driver: "postgres",
    host: sql?.host ?? "",
    port: sql?.port ?? 5432,
    database: sql?.database ?? "",
    vaultInstance: sql?.credentialSource.vaultInstance ?? "",
    credentialMount: sql?.credentialSource.mount ?? "database",
    credentialRole: sql?.credentialSource.role ?? "",
    templates: sql?.templates.map((template) => ({
      id: template.id,
      sql: template.sql,
      parameters: template.parameters.map((parameter) => ({ ...parameter })),
      timeoutSeconds: template.timeoutSeconds,
      maxRows: template.maxRows,
      maxBytes: template.maxBytes,
    })) ?? [emptySQLTemplate()],
  };
}

export function emptySQLTemplate(): SQLTemplateValues {
  return {
    id: "",
    sql: "",
    parameters: [],
    timeoutSeconds: 30,
    maxRows: 200,
    maxBytes: 65_536,
  };
}

export function sqlInstancePayload(values: SQLInstanceValues): ConnectorInstanceSaveInput {
  const body: ConnectorInstanceInput = {
    connector: "sql",
    enabled: values.enabled,
    scopeKind: values.scopeKind,
    sql: {
      driver: "postgres",
      host: values.host.trim(),
      port: values.port,
      database: values.database.trim(),
      credentialSource: {
        kind: "vault_database_role",
        vaultInstance: values.vaultInstance.trim(),
        mount: values.credentialMount.trim(),
        role: values.credentialRole.trim(),
      },
      templates: values.templates.map((template) => ({
        id: template.id.trim(),
        sql: template.sql,
        parameters: template.parameters.map((parameter) => ({
          name: parameter.name.trim(),
          type: parameter.type,
        })),
        timeoutSeconds: template.timeoutSeconds,
        maxRows: template.maxRows,
        maxBytes: template.maxBytes,
      })),
    },
  };
  applyConnectorScope(body, values);
  return { name: values.name.trim(), body };
}

export type VaultChoice = {
  name: string;
  label: string;
  ambiguous: boolean;
};

export function vaultChoices(
  instances: ConnectorInstance[],
  target: Pick<SQLInstanceValues, "scopeKind" | "company" | "area">,
): VaultChoice[] {
  const grouped = new Map<string, ConnectorInstance[]>();
  for (const instance of instances) {
    if (!covers(instance, target)) continue;
    grouped.set(instance.name, [...(grouped.get(instance.name) ?? []), instance]);
  }
  return [...grouped.entries()]
    .flatMap(([name, matches]) => {
      if (!matches.some(usableVault)) return [];
      return [{
        name,
        label: matches.length === 1 ? `${name} · ${scopeLabel(matches[0]!)}` : name,
        ambiguous: matches.length !== 1,
      }];
    })
    .sort((left, right) => left.name.localeCompare(right.name));
}

function usableVault(instance: ConnectorInstance): boolean {
  return instance.connector === "vault" && instance.enabled && instance.hasToken &&
    instance.vault?.address.startsWith("https://") === true;
}

function covers(
  source: ConnectorInstance,
  target: Pick<SQLInstanceValues, "scopeKind" | "company" | "area">,
): boolean {
  if (source.scopeKind === "installation") return true;
  if (target.scopeKind === "installation") return false;
  if (source.company !== target.company) return false;
  if (source.scopeKind === "company") return true;
  return target.scopeKind === "area" && source.area === target.area;
}

function scopeLabel(instance: ConnectorInstance): string {
  if (instance.scopeKind === "installation") return "installation";
  if (instance.scopeKind === "company") return instance.company ?? "-";
  return `${instance.company ?? "-"}/${instance.area ?? "-"}`;
}

function requireScope(values: SQLInstanceValues, ctx: z.RefinementCtx) {
  if (values.scopeKind !== "installation" && values.company.trim() === "") {
    issue(ctx, ["company"], "connectors.companyRequired");
  }
  if (values.scopeKind === "area" && values.area.trim() === "") {
    issue(ctx, ["area"], "connectors.areaRequired");
  }
}

function unique(values: string[]): string[] {
  const seen = new Set<string>();
  const duplicates = new Set<string>();
  for (const value of values) {
    if (seen.has(value)) duplicates.add(value);
    seen.add(value);
  }
  return [...duplicates];
}

function issue(ctx: z.RefinementCtx, path: (string | number)[], message: string) {
  ctx.addIssue({ code: "custom", path, message });
}
