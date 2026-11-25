import { z } from "zod";
import { githubAppIdsValid } from "@/features/integrations/mcp/github-app-credential";
import { customHeaderError } from "@/features/integrations/mcp/custom-header";

/**
 * What a tool server needs depends on how it is reached.
 *
 * The refinement is where that lives, rather than in the markup: a local
 * server without a command and a remote one without an address are the same
 * mistake, and the server refuses both — this is only what says so before the
 * round trip.
 */
export const serverSchema = z
  .object({
    name: z
      .string()
      .min(1, "integrations.nameServer")
      .regex(/^[a-z0-9][a-z0-9_-]*$/, "integrations.nameCharset"),
    transport: z.enum(["stdio", "http"]),
    // Where the server is stored (NT-019); it does not move once written.
    scopeKind: z.enum(["installation", "company", "area"]),
    company: z.string(),
    area: z.string(),
    protocolMode: z.enum(["auto", "legacy"]),
    command: z.string(),
    args: z.string(),
    url: z.string(),
    // The credential's shape, once somebody picks one; null is the one the
    // form opens on, stored or first offered. Switching clears the others, and
    // a save sends only the shape shown (formCredential).
    authChoice: z.enum(["token", "headers", "customHeader", "oauth", "githubApp"]).nullable(),
    token: z.string(),
    headers: z.record(z.string(), z.string()),
    // A header a server with no recipe is reached with, such as apikey.
    customHeaderName: z.string(),
    customHeaderValue: z.string(),
    env: z.string(),
    dsn: z.string(),
    oauthAccessToken: z.string(),
    oauthRefreshToken: z.string(),
    oauthTokenURL: z.string(),
    oauthClientID: z.string(),
    oauthClientSecret: z.string(),
    oauthTokenType: z.string(),
    oauthExpiresAtUnix: z.string(),
    oauthScopes: z.string(),
    githubAppId: z.string(),
    githubInstallationId: z.string(),
    githubPrivateKey: z.string(),
    githubRepositories: z.string(),
    githubAPIBaseURL: z.string(),
    githubToolsets: z.string(),
    configFile: z.string(),
    configFileEnv: z
      .string()
      .regex(/^[A-Za-z_][A-Za-z0-9_]*$|^$/, "mcp.configFileEnvInvalid"),
    callTimeoutSeconds: z
      .string()
      .regex(/^$|^\d+$/, "mcp.callTimeoutInvalid")
      .refine((v) => v.trim() === "" || (Number(v) >= 5 && Number(v) <= 600), {
        message: "mcp.callTimeoutInvalid",
      }),
    rateLimitPerSecond: z
      .string()
      .regex(/^$|^(?:0|[1-9]\d*)(?:\.\d+)?$/, "mcp.rateLimitRateInvalid"),
    rateLimitBurst: z
      .string()
      .regex(/^$|^\d+$/, "mcp.rateLimitBurstInvalid"),
    cacheTTLSeconds: z
      .string()
      .regex(/^$|^\d+$/, "mcp.cacheTTLInvalid"),
    cacheMaxEntries: z
      .string()
      .regex(/^$|^\d+$/, "mcp.cacheMaxEntriesInvalid"),
    stdioEgressMode: z.enum(["inherit", "proxied"]),
    stdioEgressDestinations: z.string(),
    acceptsLocalExecution: z.boolean(),
    enabled: z.boolean(),
  })
  .refine((v) => v.scopeKind === "installation" || v.company.trim() !== "", {
    path: ["company"],
    message: "connectors.companyRequired",
  })
  .refine((v) => v.scopeKind !== "area" || v.area.trim() !== "", {
    path: ["area"],
    message: "connectors.areaRequired",
  })
  /*
   * A company's or an area's server is remote: a local one runs on the
   * platform's worker, and is the platform's. The server refuses the pair;
   * the form offers only HTTP there, and this holds if it ever did not.
   */
  .refine((v) => v.scopeKind === "installation" || v.transport === "http", {
    path: ["transport"],
    message: "integrations.remoteOutsideInstallation",
  })
  .refine((v) => v.transport !== "stdio" || v.command.trim() !== "", {
    path: ["command"],
    message: "agents.sayWhatToRun",
  })
  .refine((v) => v.transport !== "http" || v.url.trim() !== "", {
    path: ["url"],
    message: "integrations.sayWhereToCall",
  })
  .refine(
    (v) =>
      v.transport !== "http" ||
      v.oauthExpiresAtUnix.trim() === "" ||
      /^\d+$/.test(v.oauthExpiresAtUnix.trim()),
    {
      path: ["oauthExpiresAtUnix"],
      message: "mcp.oauthExpiryInvalid",
    },
  )
  .superRefine((v, ctx) => {
    if (v.transport !== "http") return;
    const header = { name: v.customHeaderName, value: v.customHeaderValue };
    const error = customHeaderError(header);
    if (error) ctx.addIssue({ code: "custom", path: ["customHeaderName"], message: error });
  })
  .refine((v) => v.transport !== "http" || githubAppIdsValid(v), {
    path: ["githubAppId"],
    message: "mcp.githubAppIdsInvalid",
  })
  .refine((v) => rateLimitComplete(v), {
    path: ["rateLimitPerSecond"],
    message: "mcp.rateLimitPairInvalid",
  })
  .refine((v) => resultCacheComplete(v), {
    path: ["cacheTTLSeconds"],
    message: "mcp.cachePairInvalid",
  })
  .refine((v) => stdioEgressComplete(v), {
    path: ["stdioEgressDestinations"],
    message: "mcp.stdioEgressDestinationsInvalid",
  })
  /*
   * A local server is a program this installation starts inside the worker.
   * The server refuses one nobody accepted; this says so before the round
   * trip, and beside the box rather than in a toast afterwards.
   */
  .refine((v) => v.transport !== "stdio" || v.acceptsLocalExecution, {
    path: ["acceptsLocalExecution"],
    message: "integrations.acceptLocalExecutionRequired",
  });

export type ServerFormValues = z.infer<typeof serverSchema>;

function rateLimitComplete(values: {
  rateLimitPerSecond: string;
  rateLimitBurst: string;
}) {
  const rate = positiveNumber(values.rateLimitPerSecond);
  const burst = positiveInteger(values.rateLimitBurst);
  if (rate && burst) return true;
  return disabledNumber(values.rateLimitPerSecond) && disabledNumber(values.rateLimitBurst);
}

function resultCacheComplete(values: {
  cacheTTLSeconds: string;
  cacheMaxEntries: string;
}) {
  if (positiveInteger(values.cacheTTLSeconds)) return true;
  return disabledNumber(values.cacheTTLSeconds) && disabledNumber(values.cacheMaxEntries);
}

function stdioEgressComplete(values: {
  transport: "stdio" | "http";
  stdioEgressMode: "inherit" | "proxied";
  stdioEgressDestinations: string;
}) {
  if (values.transport !== "stdio" || values.stdioEgressMode === "inherit") {
    return true;
  }
  const lines = values.stdioEgressDestinations
    .split(/\r?\n/)
    .map((line) => line.trim())
    .filter(Boolean);
  return lines.length > 0 && lines.every(validEgressDestinationLine);
}

function validEgressDestinationLine(line: string) {
  const at = line.lastIndexOf(":");
  if (at <= 0 || at === line.length - 1) return false;
  const host = line.slice(0, at).toLowerCase();
  const port = Number.parseInt(line.slice(at + 1), 10);
  if (!Number.isInteger(port) || port < 1 || port > 65535) return false;
  return /^(?:\*\.)?[a-z0-9](?:[a-z0-9_.-]{0,251}[a-z0-9])?$/.test(host);
}

function positiveNumber(raw: string) {
  return raw.trim() !== "" && Number(raw) > 0;
}

function positiveInteger(raw: string) {
  return raw.trim() !== "" && Number(raw) > 0;
}

function disabledNumber(raw: string) {
  return raw.trim() === "" || Number(raw) === 0;
}
