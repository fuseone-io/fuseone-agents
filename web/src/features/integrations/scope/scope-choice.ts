import { createContext } from "react";
import type { MeScope } from "@/features/session/api";

/**
 * Where the forms below may place what they write (NT-019): a connector
 * instance or an MCP server.
 *
 * Provided by the screen, from the session it already read, so a form asks
 * the server for nothing. Without a provider the scope is free, as it was
 * before areas configured their own integrations; the server still refuses a
 * scope the caller may not write.
 */
export interface ScopeChoice {
  anyScope: boolean;
  configure: MeScope[];
}

export const ScopeChoiceContext = createContext<ScopeChoice>({
  anyScope: true,
  configure: [],
});

/** What a form holding a scope field stores: the row's own scope. */
export type ScopeValues = {
  scopeKind: "installation" | "company" | "area";
  company: string;
  area: string;
};
