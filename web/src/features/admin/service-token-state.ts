import type { ServiceToken } from "@/features/admin/service-accounts-api";

export type TokenState = "active" | "expired" | "revoked";

/** Revoked outranks expired: it is what somebody did, and it stays true. */
export function tokenState(token: ServiceToken, now = Date.now()): TokenState {
  if (token.revokedAt) return "revoked";
  return Date.parse(token.expiresAt) <= now ? "expired" : "active";
}

export function liveTokens(tokens: ServiceToken[], now = Date.now()) {
  return tokens.filter((token) => tokenState(token, now) === "active");
}
