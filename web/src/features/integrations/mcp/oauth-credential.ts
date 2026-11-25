import type { MCPOAuthGrant } from "@/features/integrations/api";

export type OAuthCredentialInput = {
  oauthAccessToken: string;
  oauthRefreshToken: string;
  oauthTokenURL: string;
  oauthClientID: string;
  oauthClientSecret: string;
  oauthTokenType: string;
  oauthExpiresAtUnix: string;
  oauthScopes: string;
};

export function oauthHasValue(value: OAuthCredentialInput) {
  return [
    value.oauthAccessToken,
    value.oauthRefreshToken,
    value.oauthTokenURL,
    value.oauthClientID,
    value.oauthClientSecret,
    value.oauthTokenType,
    value.oauthExpiresAtUnix,
    value.oauthScopes,
  ].some((part) => part.trim() !== "");
}

/**
 * Whether a request could carry the grant: an access token, or a refresh
 * token with the token URL to renew one at. The server refuses any other
 * grant that holds something, and so does the worker.
 */
export function oauthIsUsable(value: OAuthCredentialInput) {
  const present = (part: string) => part.trim() !== "";
  return present(value.oauthAccessToken) || (present(value.oauthRefreshToken) && present(value.oauthTokenURL));
}

export function oauthExpiryIsValid(value: OAuthCredentialInput) {
  const raw = value.oauthExpiresAtUnix.trim();
  return raw === "" || /^\d+$/.test(raw);
}

export function oauthFromValue(value: OAuthCredentialInput): MCPOAuthGrant {
  const expires = value.oauthExpiresAtUnix.trim();
  const scopes = value.oauthScopes
    .split(/\s+/)
    .map((scope) => scope.trim())
    .filter(Boolean);
  return {
    accessToken: emptyAsUndefined(value.oauthAccessToken),
    refreshToken: emptyAsUndefined(value.oauthRefreshToken),
    tokenURL: emptyAsUndefined(value.oauthTokenURL),
    clientID: emptyAsUndefined(value.oauthClientID),
    clientSecret: emptyAsUndefined(value.oauthClientSecret),
    tokenType: emptyAsUndefined(value.oauthTokenType),
    expiresAtUnix: expires === "" ? undefined : Number.parseInt(expires, 10),
    scopes: scopes.length === 0 ? undefined : scopes,
  };
}

function emptyAsUndefined(value: string) {
  const trimmed = value.trim();
  return trimmed === "" ? undefined : trimmed;
}
