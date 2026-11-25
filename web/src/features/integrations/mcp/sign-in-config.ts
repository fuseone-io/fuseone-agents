import { z } from "zod";
import type { MCPOAuthLogin, MCPOAuthLoginInput } from "@/features/integrations/api";

/** How the token endpoint is told which client asks; auto is the server's default. */
export const TOKEN_AUTH_METHODS = ["auto", "none", "client_secret_post", "client_secret_basic"] as const;

export const signInConfigSchema = z.object({
  clientID: z.string().trim().min(1, "mcp.signIn.clientRequired"),
  clientSecret: z.string(),
  scopes: z.string(),
  authorizeURL: z.string().trim(),
  tokenURL: z.string().trim(),
  resource: z.string().trim(),
  tokenAuthMethod: z.enum(TOKEN_AUTH_METHODS),
});

export type SignInConfigValues = z.infer<typeof signInConfigSchema>;

/** The form opens on what is stored: the configuration is written whole. */
export function signInDefaults(stored: MCPOAuthLogin | undefined): SignInConfigValues {
  return {
    clientID: stored?.clientID ?? "",
    clientSecret: "",
    scopes: (stored?.scopes ?? []).join(" "),
    authorizeURL: stored?.authorizeURL ?? "",
    tokenURL: stored?.tokenURL ?? "",
    resource: stored?.resource ?? "",
    tokenAuthMethod: stored?.tokenAuthMethod ?? "auto",
  };
}

/**
 * What a save sends: the configuration whole, the fields left empty out of
 * it, and the client secret only when one was typed. Left blank, the stored
 * secret is kept; removing it is its own gesture (withoutSecret).
 */
export function signInInput(values: SignInConfigValues): MCPOAuthLoginInput {
  const input: MCPOAuthLoginInput = { clientID: values.clientID.trim() };
  const scopes = values.scopes.split(/\s+/).filter(Boolean);
  if (scopes.length > 0) input.scopes = scopes;
  if (values.authorizeURL) input.authorizeURL = values.authorizeURL;
  if (values.tokenURL) input.tokenURL = values.tokenURL;
  if (values.resource) input.resource = values.resource;
  if (values.tokenAuthMethod !== "auto") input.tokenAuthMethod = values.tokenAuthMethod;
  if (values.clientSecret !== "") input.clientSecret = values.clientSecret;
  return input;
}

/**
 * The stored configuration with its secret removed. The method is left to the
 * server's default: one that sends a secret, with none left, would be refused.
 */
export function withoutSecret(stored: MCPOAuthLogin): MCPOAuthLoginInput {
  const input = signInInput({ ...signInDefaults(stored), tokenAuthMethod: "auto" });
  input.clientSecret = "";
  return input;
}

/** Where the provider sends people back to: this installation's own address. */
export function signInCallbackURL(origin: string) {
  return `${origin}/auth/mcp/callback`;
}
