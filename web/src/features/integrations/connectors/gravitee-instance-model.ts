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
import {
  usableVault,
  vaultOwnsPath,
  vaultsNamed,
  type ConnectorTarget,
} from "@/features/integrations/connectors/vault-instance-options";

const graviteeID = /^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$/;
const vaultPath = /^[A-Za-z0-9_.-]+(?:\/[A-Za-z0-9_.-]+)*$/;
const vaultField = /^[A-Za-z0-9_.-]{1,64}$/;
const maxTTLSeconds = 365 * 24 * 60 * 60;

export const graviteeInstanceSchema = z
  .object({
    name: z.string().regex(connectorInstanceName, "connectors.instanceNameInvalid"),
    enabled: z.boolean(),
    scopeKind: z.enum(["installation", "company", "area"]),
    company: z.string(),
    area: z.string(),
    address: z.string().trim().refine(validGraviteeAddress, "connectors.graviteeAddressInvalid"),
    organization: z.string().regex(graviteeID, "connectors.graviteeOrganizationInvalid"),
    environment: z.string().regex(graviteeID, "connectors.graviteeEnvironmentInvalid"),
    apiReference: z.string().regex(graviteeID, "connectors.graviteeAPIInvalid"),
    minTTLSeconds: z.number({ invalid_type_error: "connectors.graviteeTTLInvalid" })
      .int().min(1, "connectors.graviteeTTLInvalid")
      .max(maxTTLSeconds, "connectors.graviteeTTLInvalid"),
    maxTTLSeconds: z.number({ invalid_type_error: "connectors.graviteeTTLInvalid" })
      .int().min(1, "connectors.graviteeTTLInvalid")
      .max(maxTTLSeconds, "connectors.graviteeTTLInvalid"),
    allowNoExpiry: z.boolean(),
    vaultInstance: z.string().regex(connectorInstanceName, "connectors.graviteeVaultRequired"),
    credentialPath: z.string().max(512, "connectors.graviteeCredentialPathInvalid").refine(
      validVaultPath,
      "connectors.graviteeCredentialPathInvalid",
    ),
    credentialField: z.string().regex(vaultField, "connectors.graviteeCredentialFieldInvalid"),
  })
  .superRefine((values, ctx) => {
    requireScope(values, ctx);
    if (values.maxTTLSeconds < values.minTTLSeconds) {
      issue(ctx, ["maxTTLSeconds"], "connectors.graviteeTTLInverted");
    }
  });

export type GraviteeInstanceValues = z.infer<typeof graviteeInstanceSchema>;

export function graviteeInstanceDefaults(
  instance: ConnectorInstanceDetail | null,
  company: string,
  area: string,
): GraviteeInstanceValues {
  const gravitee = instance?.gravitee;
  return {
    name: instance?.name ?? "",
    enabled: instance?.enabled ?? true,
    scopeKind: instance?.scopeKind ?? "area",
    company: instance?.company ?? concreteScope(company),
    area: instance?.area ?? concreteScope(area),
    address: gravitee?.address ?? "",
    organization: gravitee?.organization ?? "",
    environment: gravitee?.environment ?? "",
    apiReference: gravitee?.allowedReferences[0]?.id ?? "",
    minTTLSeconds: gravitee?.minTTLSeconds ?? 86_400,
    maxTTLSeconds: gravitee?.maxTTLSeconds ?? 7_776_000,
    allowNoExpiry: gravitee?.allowNoExpiry ?? false,
    vaultInstance: gravitee?.credentialSource.vaultInstance ?? "",
    credentialPath: gravitee?.credentialSource.path ?? "",
    credentialField: gravitee?.credentialSource.field ?? "access_token",
  };
}

export function graviteeInstancePayload(
  values: GraviteeInstanceValues,
): ConnectorInstanceSaveInput {
  const body: ConnectorInstanceInput = {
    connector: "gravitee",
    enabled: values.enabled,
    scopeKind: values.scopeKind,
    gravitee: {
      address: values.address.trim(),
      organization: values.organization.trim(),
      environment: values.environment.trim(),
      allowedReferences: [{ type: "API", id: values.apiReference.trim() }],
      minTTLSeconds: values.minTTLSeconds,
      maxTTLSeconds: values.maxTTLSeconds,
      allowNoExpiry: values.allowNoExpiry,
      credentialSource: {
        kind: "vault_kv_secret",
        vaultInstance: values.vaultInstance.trim(),
        path: values.credentialPath.trim(),
        field: values.credentialField.trim(),
      },
    },
  };
  applyConnectorScope(body, values);
  return { name: values.name.trim(), body };
}

export function graviteeBindingIssue(
  instances: ConnectorInstance[],
  target: ConnectorTarget,
  name: string,
  path: string,
): string | null {
  const matches = vaultsNamed(instances, target, name);
  if (matches.length > 1) return "connectors.graviteeVaultAmbiguous";
  const vault = matches[0];
  if (!vault || !usableVault(vault)) return "connectors.graviteeVaultUnavailable";
  if (!vaultOwnsPath(vault, path)) return "connectors.graviteeVaultPathOutside";
  return null;
}

function validGraviteeAddress(raw: string): boolean {
  try {
    const parsed = new URL(raw);
    if (!raw.toLowerCase().startsWith("https://") || parsed.protocol !== "https:" ||
      parsed.username || parsed.password ||
      parsed.search || parsed.hash || blockedLiteral(parsed.hostname)) {
      return false;
    }
    return !relativePath(raw);
  } catch {
    return false;
  }
}

function relativePath(raw: string): boolean {
  const afterAuthority = raw.slice("https://".length);
  const slash = afterAuthority.indexOf("/");
  if (slash < 0) return false;
  const encoded = afterAuthority.slice(slash).split(/[?#]/, 1)[0]!;
  try {
    const decoded = decodeURIComponent(encoded);
    return decoded.includes("\0") || decoded.split("/").some((part) => part === "." || part === "..");
  } catch {
    return true;
  }
}

function blockedLiteral(raw: string): boolean {
  const host = raw.replace(/^\[|\]$/g, "").toLowerCase();
  if (host.startsWith("169.254.")) return true;
  if (host.startsWith("::ffff:a9fe:")) return true;
  if (host.startsWith("fd00:ec2:")) return true;
  return /^fe[89ab][0-9a-f]:/.test(host);
}

function concreteScope(value: string): string {
  return value === "*" ? "" : value;
}

function validVaultPath(raw: string): boolean {
  return vaultPath.test(raw) && !raw.split("/").some((part) => part === "." || part === "..");
}

function requireScope(values: GraviteeInstanceValues, ctx: z.RefinementCtx) {
  if (values.scopeKind !== "installation" && values.company.trim() === "") {
    issue(ctx, ["company"], "connectors.companyRequired");
  }
  if (values.scopeKind === "area" && values.area.trim() === "") {
    issue(ctx, ["area"], "connectors.areaRequired");
  }
}

function issue(ctx: z.RefinementCtx, path: string[], message: string) {
  ctx.addIssue({ code: "custom", path, message });
}
