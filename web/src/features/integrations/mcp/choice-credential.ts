import type { AuthChoice } from "@/features/integrations/mcp/auth-choice";
import type { RemoteAuthPlan } from "@/features/integrations/mcp/auth-plan";
import {
  remoteCredential,
  type CredentialValue,
} from "@/features/integrations/mcp/credential-value";
import {
  oauthExpiryIsValid,
  oauthFromValue,
  oauthHasValue,
} from "@/features/integrations/mcp/oauth-credential";
import type { MCPOAuthGrant } from "@/features/integrations/api";

export type ChosenCredential = {
  token?: string;
  headers?: Record<string, string>;
  oauth?: MCPOAuthGrant;
};

/**
 * What a save sends for the shape chosen, and nothing of the others.
 *
 * A value typed under another choice before switching stays in the form's
 * state; it is never written. The server replaces the other shapes when this
 * one is stored, so no removal of them has to be sent.
 */
export function chosenCredential(
  choice: AuthChoice | null,
  value: CredentialValue,
  plan: RemoteAuthPlan,
  headers: string[],
): ChosenCredential {
  switch (choice) {
    case "token":
      return remoteCredential({ ...value, headers: {} }, plan, []);
    case "headers":
      return remoteCredential({ ...value, token: "" }, plan, headers);
    case "oauth":
      return { oauth: oauthFromValue(value) };
    default:
      return {};
  }
}

/** Whether the shape chosen has what a save needs. */
export function chosenReady(
  choice: AuthChoice | null,
  value: CredentialValue,
  headers: string[],
): boolean {
  switch (choice) {
    case "token":
      return value.token.trim() !== "";
    case "headers":
      return headers.length > 0 && headers.every((header) => (value.headers[header] ?? "").trim() !== "");
    case "oauth":
      return oauthHasValue(value) && oauthExpiryIsValid(value);
    default:
      return false;
  }
}
