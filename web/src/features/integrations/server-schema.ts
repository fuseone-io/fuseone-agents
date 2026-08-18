import { z } from "zod";

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
    command: z.string(),
    args: z.string(),
    url: z.string(),
    token: z.string(),
    headers: z.record(z.string(), z.string()),
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
    configFile: z.string(),
    configFileEnv: z
      .string()
      .regex(/^[A-Za-z_][A-Za-z0-9_]*$|^$/, "mcp.configFileEnvInvalid"),
    acceptsLocalExecution: z.boolean(),
    enabled: z.boolean(),
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
  .refine(
    (v) =>
      v.transport !== "http" ||
      v.token.trim() === "" ||
      Object.values(v.headers).every((part) => part.trim() === ""),
    {
      path: ["token"],
      message: "mcp.remoteCredentialConflict",
    },
  )
  .refine(
    (v) =>
      v.transport !== "http" ||
      (v.token.trim() === "" && Object.values(v.headers).every((part) => part.trim() === "")) ||
      [
        v.oauthAccessToken,
        v.oauthRefreshToken,
        v.oauthTokenURL,
        v.oauthClientID,
        v.oauthClientSecret,
        v.oauthTokenType,
        v.oauthExpiresAtUnix,
        v.oauthScopes,
      ].every((part) => part.trim() === ""),
    {
      path: ["token"],
      message: "mcp.oauthBearerConflict",
    },
  )
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
