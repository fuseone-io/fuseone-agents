import { z } from "zod";
import type {
  ConnectorInstance,
  ConnectorInstanceInput,
} from "@/features/integrations/api";
import {
  applyConnectorScope,
  connectorInstanceName,
  type ConnectorInstanceSaveInput,
} from "@/features/integrations/connectors/connector-instance-model";

/**
 * One governed window over one index pattern. The index and the field
 * mapping are configuration — the model only ever supplies values to named
 * queries — and the password is the sealed token, never prefilled.
 */
export const elasticsearchInstanceSchema = z
  .object({
    name: z
      .string()
      .regex(connectorInstanceName, "connectors.instanceNameInvalid"),
    enabled: z.boolean(),
    scopeKind: z.enum(["installation", "company", "area"]),
    company: z.string(),
    area: z.string(),
    baseUrl: z.string().regex(/^https?:\/\/.+/, "connectors.esBaseUrlInvalid"),
    username: z.string().min(1, "connectors.esUsernameRequired"),
    index: z
      .string()
      .regex(/^[a-z0-9][a-z0-9*._-]{0,254}$/, "connectors.esIndexInvalid"),
    timestampField: z.string(),
    ipField: z.string(),
    pathField: z.string(),
    statusField: z.string(),
    maxWindowDays: z.coerce
      .number()
      .int("connectors.esWindowInvalid")
      .min(0, "connectors.esWindowInvalid")
      .max(30, "connectors.esWindowInvalid"),
    token: z.string(),
    clearToken: z.boolean(),
  })
  .superRefine((values, ctx) => {
    if (values.scopeKind !== "installation" && values.company.trim() === "") {
      ctx.addIssue({
        code: "custom",
        path: ["company"],
        message: "connectors.companyRequired",
      });
    }
    if (values.scopeKind === "area" && values.area.trim() === "") {
      ctx.addIssue({
        code: "custom",
        path: ["area"],
        message: "connectors.areaRequired",
      });
    }
  });

export type ElasticsearchInstanceValues = z.infer<
  typeof elasticsearchInstanceSchema
>;

export function elasticsearchInstanceDefaults(
  instance: ConnectorInstance | null,
  company: string,
  area: string,
): ElasticsearchInstanceValues {
  return {
    name: instance?.name ?? "",
    enabled: instance?.enabled ?? true,
    scopeKind: instance?.scopeKind ?? "area",
    company: instance?.company ?? company,
    area: instance?.area ?? area,
    baseUrl: instance?.elasticsearch?.baseUrl ?? "",
    username: instance?.elasticsearch?.username ?? "",
    index: instance?.elasticsearch?.index ?? "",
    timestampField: instance?.elasticsearch?.timestampField ?? "",
    ipField: instance?.elasticsearch?.ipField ?? "",
    pathField: instance?.elasticsearch?.pathField ?? "",
    statusField: instance?.elasticsearch?.statusField ?? "",
    maxWindowDays: instance?.elasticsearch?.maxWindowDays ?? 0,
    token: "",
    clearToken: false,
  };
}

export function elasticsearchInstancePayload(
  values: ElasticsearchInstanceValues,
  hasStoredToken: boolean,
): ConnectorInstanceSaveInput | null {
  if (values.enabled && !hasStoredToken && values.token.trim() === "") {
    return null;
  }
  if (values.enabled && values.clearToken && values.token.trim() === "") {
    return null;
  }
  const body: ConnectorInstanceInput = {
    connector: "elasticsearch",
    enabled: values.enabled,
    scopeKind: values.scopeKind,
    elasticsearch: {
      baseUrl: values.baseUrl.trim(),
      username: values.username.trim(),
      index: values.index.trim(),
    },
  };
  applyConnectorScope(body, values);
  const es = body.elasticsearch!;
  if (values.timestampField.trim())
    es.timestampField = values.timestampField.trim();
  if (values.ipField.trim()) es.ipField = values.ipField.trim();
  if (values.pathField.trim()) es.pathField = values.pathField.trim();
  if (values.statusField.trim()) es.statusField = values.statusField.trim();
  if (values.maxWindowDays > 0) es.maxWindowDays = values.maxWindowDays;
  if (values.token.trim()) body.token = values.token;
  if (values.clearToken && !body.token) body.clearToken = true;
  return { name: values.name.trim(), body };
}
