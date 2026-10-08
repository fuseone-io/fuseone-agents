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
 * One Cloudflare IP list a firewall rule blocks at the edge. Everything an
 * installation must never block — its own egress, an anonymizer range whose
 * addresses are shared by many real clients — is typed here as CIDRs, one
 * per line, and the connector refuses those on top of the private ranges it
 * always refuses in code.
 */
export const cloudflareInstanceSchema = z
  .object({
    name: z
      .string()
      .regex(connectorInstanceName, "connectors.instanceNameInvalid"),
    enabled: z.boolean(),
    scopeKind: z.enum(["installation", "company", "area"]),
    company: z.string(),
    area: z.string(),
    baseUrl: z.string(),
    accountId: z
      .string()
      .regex(/^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$/, "connectors.accountIdInvalid"),
    listId: z
      .string()
      .regex(/^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$/, "connectors.listIdInvalid"),
    protectedRanges: z.string(),
    maxBlocksPerDay: z.coerce
      .number()
      .int("connectors.maxBlocksPerDayInvalid")
      .min(0, "connectors.maxBlocksPerDayInvalid")
      .max(10000, "connectors.maxBlocksPerDayInvalid"),
    token: z.string(),
    clearToken: z.boolean(),
  })
  .superRefine((values, ctx) => {
    if (
      values.baseUrl.trim() !== "" &&
      !/^https:\/\//.test(values.baseUrl.trim())
    ) {
      ctx.addIssue({
        code: "custom",
        path: ["baseUrl"],
        message: "connectors.baseUrlInvalid",
      });
    }
    for (const range of cloudflareRanges(values.protectedRanges)) {
      if (!looksLikeCIDR(range)) {
        ctx.addIssue({
          code: "custom",
          path: ["protectedRanges"],
          message: "connectors.protectedRangesInvalid",
        });
        break;
      }
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

export type CloudflareInstanceValues = z.infer<typeof cloudflareInstanceSchema>;

// A sanity shape, not a parser: the server validates each range for real.
// What this catches on screen is a bare address or plain text, because a
// protected set wants to be explicit about its width.
function looksLikeCIDR(range: string): boolean {
  return /^[0-9a-fA-F:.]+\/\d{1,3}$/.test(range);
}

export function cloudflareRanges(raw: string): string[] {
  return raw
    .split(/[\n,]/)
    .map((part) => part.trim())
    .filter(Boolean);
}

export function cloudflareInstanceDefaults(
  instance: ConnectorInstance | null,
  company: string,
  area: string,
): CloudflareInstanceValues {
  return {
    name: instance?.name ?? "",
    enabled: instance?.enabled ?? true,
    scopeKind: instance?.scopeKind ?? "area",
    company: instance?.company ?? company,
    area: instance?.area ?? area,
    baseUrl: instance?.cloudflare?.baseUrl ?? "",
    accountId: instance?.cloudflare?.accountId ?? "",
    listId: instance?.cloudflare?.listId ?? "",
    protectedRanges: (instance?.cloudflare?.protectedRanges ?? []).join("\n"),
    maxBlocksPerDay: instance?.cloudflare?.maxBlocksPerDay ?? 0,
    // The token is never prefilled: what is stored stays stored unless a new
    // value is typed or the clear switch says to remove it.
    token: "",
    clearToken: false,
  };
}

export function cloudflareInstancePayload(
  values: CloudflareInstanceValues,
  hasStoredToken: boolean,
): ConnectorInstanceSaveInput | null {
  if (values.enabled && !hasStoredToken && values.token.trim() === "") {
    return null;
  }
  if (values.enabled && values.clearToken && values.token.trim() === "") {
    return null;
  }
  const body: ConnectorInstanceInput = {
    connector: "cloudflare",
    enabled: values.enabled,
    scopeKind: values.scopeKind,
    cloudflare: {
      accountId: values.accountId.trim(),
      listId: values.listId.trim(),
    },
  };
  applyConnectorScope(body, values);
  if (values.baseUrl.trim()) body.cloudflare!.baseUrl = values.baseUrl.trim();
  const ranges = cloudflareRanges(values.protectedRanges);
  if (ranges.length > 0) body.cloudflare!.protectedRanges = ranges;
  if (values.maxBlocksPerDay > 0) {
    body.cloudflare!.maxBlocksPerDay = values.maxBlocksPerDay;
  }
  if (values.token.trim()) body.token = values.token;
  if (values.clearToken && !body.token) body.clearToken = true;
  return { name: values.name.trim(), body };
}
