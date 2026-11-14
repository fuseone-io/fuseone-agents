import type { MCPCredentialServer, MCPServer } from "@/features/integrations/api";

/** What the personal credentials read of a server: never more than its name, reach and whether it holds a credential. */
export type CredentialServer = Pick<MCPServer, "name" | "transport" | "url" | "hasSecret" | "hasOAuth">;

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
  if (listed) return listed;
  return (offered ?? []).map((server) => ({
    name: server.name,
    transport: "http",
    hasSecret: server.shared,
  }));
}
