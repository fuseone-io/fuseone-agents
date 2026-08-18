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
