import type { Me, MeScope } from "@/features/session/api";

/**
 * What the caller may open and change under integrations (NT-019).
 *
 * Connector instances are configured at their own scope, so an area's
 * administrator works on them. MCP servers, providers and channels are still
 * the platform's: they open and change only for what is held there. A server
 * that predates `platform` answers for `can` alone, as before.
 */
export interface IntegrationAccess {
  /** The session has answered. Until it does, nothing of the platform is asked for. */
  known: boolean;
  /** Opens every integrations screen. */
  platform: boolean;
  /** Changes MCP servers, providers and channels. */
  platformConfigure: boolean;
  /** Opens the connector instances. */
  connectors: boolean;
  /** May create a connector instance somewhere. */
  createConnector: boolean;
  /** Governs every scope: the connector form leaves the scope free. */
  anyScope: boolean;
  /** Where a connector instance may be created when anyScope is false. */
  configure: MeScope[];
}

export function integrationAccess(me: Me | null | undefined): IntegrationAccess {
  if (me === null) return open;
  const can = me?.can ?? [];
  const platform = me?.platform;
  const writes = can.includes("provider:write");
  const atPlatform = (perm: string) => (platform ? platform.includes(perm) : can.includes(perm));
  const known = me !== undefined;
  const opensAll = known && (!platform || platform.includes("tool:read"));
  return {
    known,
    platform: opensAll,
    platformConfigure: atPlatform("provider:write"),
    connectors: opensAll || writes,
    createConnector: writes,
    // Before /me answers, the form leaves the scope free; the server decides.
    anyScope: !me || atPlatform("provider:write"),
    configure: me?.configure ?? [],
  };
}

const open: IntegrationAccess = {
  known: true,
  platform: true,
  platformConfigure: true,
  connectors: true,
  createConnector: true,
  anyScope: true,
  configure: [],
};
