import { createContext } from "react";
import type { MeScope } from "@/features/session/api";

/**
 * Where the connector forms below may place an instance (NT-019).
 *
 * Provided by the connectors screen, from the session it already read, so a
 * form asks the server for nothing. Without a provider the scope is free, as
 * it was before areas configured their own connectors; the server still
 * refuses a scope the caller may not write.
 */
export interface ConnectorScopeChoice {
  anyScope: boolean;
  configure: MeScope[];
}

export const ConnectorScopeContext = createContext<ConnectorScopeChoice>({
  anyScope: true,
  configure: [],
});
