import type { MCPCredentialServer, MCPServer } from "@/features/integrations/api";

/** What the personal credentials read of a server: never more than its name, reach and whether it holds a credential. */
export type CredentialServer = Pick<MCPServer, "name" | "transport" | "url" | "hasSecret" | "hasOAuth"> & {
  /** People sign in to it from their browser. */
  signIn?: boolean;
  /** Every tool call takes the person's own credential: its recipe says so, or people sign in to it. */
  requiresPersonal?: boolean;
};

/**
 * The servers a person keeps a credential for (NT-019). Whoever configures
 * servers reads them from the servers' listing, address included. Anyone
 * else, an area's author, reads them from the credentials' own answer, which
 * names no address: each one is remote, and `shared` says it holds a
 * credential of its own.
 */
export function credentialServers(
  listed: MCPServer[] | undefined,
  offered: MCPCredentialServer[] | undefined,
): CredentialServer[] {
  if (listed) {
    return listed.map((server) => ({
      ...server,
      signIn: server.oauthLogin !== undefined,
      requiresPersonal: server.callAuth?.policy === "personal_required",
    }));
  }
  return (offered ?? []).map((server) => ({
    name: server.name,
    transport: "http",
    hasSecret: server.shared,
    signIn: server.signIn,
    requiresPersonal: server.requiresPersonal,
  }));
}
