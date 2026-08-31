import { z } from "zod";
import type {
  ConnectorInstance,
  ConnectorInstanceInput,
  ConnectorScopeKind,
} from "@/features/integrations/api";

export const connectorInstanceName = /^[a-z0-9][a-z0-9_-]{0,62}$/;

export const connectorInstanceSchema = z
  .object({
    name: z.string().regex(connectorInstanceName, "connectors.instanceNameInvalid"),
    enabled: z.boolean(),
    scopeKind: z.enum(["installation", "company", "area"]),
    company: z.string(),
    area: z.string(),
    address: z.string().url("connectors.addressInvalid"),
    mount: z.string().min(1, "connectors.mountRequired"),
    namespace: z.string(),
    allowedPathPrefixes: z.string().min(1, "connectors.pathPrefixesRequired"),
    token: z.string(),
    clearToken: z.boolean(),
  })
  .superRefine((values, ctx) => {
    if (pathPrefixes(values.allowedPathPrefixes).length === 0) {
      ctx.addIssue({
        code: "custom",
        path: ["allowedPathPrefixes"],
        message: "connectors.pathPrefixesRequired",
      });
    }
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

export type ConnectorInstanceValues = z.infer<typeof connectorInstanceSchema>;
export type ConnectorInstanceSaveInput = {
  name: string;
  body: ConnectorInstanceInput;
};
export type ConnectorInstanceSaver = (
  input: ConnectorInstanceSaveInput,
) => Promise<void>;

export function connectorInstanceDefaults(
  instance: ConnectorInstance | null,
  company: string,
  area: string,
): ConnectorInstanceValues {
  return {
    name: instance?.name ?? "",
    enabled: instance?.enabled ?? true,
    scopeKind: instance?.scopeKind ?? "area",
    company: instance?.company ?? company,
    area: instance?.area ?? area,
    address: instance?.vault?.address ?? "",
    mount: instance?.vault?.mount ?? "secret",
    namespace: instance?.vault?.namespace ?? "",
    allowedPathPrefixes: (instance?.vault?.allowedPathPrefixes ?? []).join("\n"),
    token: "",
    clearToken: false,
  };
}

export function connectorInstancePayload(
  values: ConnectorInstanceValues,
  hasStoredToken: boolean,
): ConnectorInstanceSaveInput | null {
  const prefixes = pathPrefixes(values.allowedPathPrefixes);
  if (prefixes.length === 0) return null;
  if (values.enabled && !hasStoredToken && values.token.trim() === "") return null;
  if (values.enabled && values.clearToken && values.token.trim() === "") return null;

  const body: ConnectorInstanceInput = {
    connector: "vault",
    enabled: values.enabled,
    scopeKind: values.scopeKind,
    vault: {
      address: values.address.trim(),
      mount: values.mount.trim(),
      allowedPathPrefixes: prefixes,
    },
  };
  applyConnectorScope(body, values);
  if (values.namespace.trim()) body.vault!.namespace = values.namespace.trim();
  if (values.token.trim()) body.token = values.token;
  if (values.clearToken && !body.token) body.clearToken = true;
  return { name: values.name.trim(), body };
}

export function pathPrefixes(raw: string): string[] {
  return raw
    .split(/[\n,]/)
    .map((part) => part.trim())
    .filter(Boolean);
}

export function applyConnectorScope(
  body: ConnectorInstanceInput,
  values: { scopeKind: ConnectorScopeKind; company: string; area: string },
) {
  if (values.scopeKind === "installation") return;
  body.company = values.company.trim();
  if (values.scopeKind === "area") body.area = values.area.trim();
}
