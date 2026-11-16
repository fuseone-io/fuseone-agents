import { z } from "zod";
import { useActiveScope } from "@/features/scope/active-scope";
import type { components } from "@/lib/api/schema.gen";

/** The company the active scope names for the installation itself. */
const INSTALLATION_COMPANY = "*";

/**
 * Where a connection is stored (NT-019, phase 4 writers): an area's
 * administrator stores it in a scope they configure; the platform anywhere,
 * the installation by default.
 */
export const channelScopeShape = {
  scopeKind: z.enum(["installation", "company", "area"]),
  company: z.string(),
  area: z.string(),
};

type Scoped = { scopeKind: "installation" | "company" | "area"; company: string; area: string };

/** A company is named for a company's or an area's connection, an area for an area's. */
export function scopeIsComplete<T extends z.ZodType<Scoped>>(schema: T): T {
  return schema
    .refine((v) => v.scopeKind === "installation" || v.company.trim() !== "", {
      path: ["company"],
      message: "connectors.companyRequired",
    })
    .refine((v) => v.scopeKind !== "area" || v.area.trim() !== "", {
      path: ["area"],
      message: "connectors.areaRequired",
    });
}

/**
 * Where a connection's form starts. A stored one, where it is; a new one in
 * the scope the console is reading, as a new server and a new provider do, so
 * the app reaches no more than was asked.
 */
export function useChannelScopeDefaults(channel: components["schemas"]["Channel"] | null): Scoped {
  const activeCompany = useActiveScope((state) => state.company);
  const activeArea = useActiveScope((state) => state.area);
  if (channel) {
    return { scopeKind: channel.scopeKind ?? "installation", company: channel.company ?? "", area: channel.area ?? "" };
  }
  const reading =
    activeCompany === INSTALLATION_COMPANY ? { company: "", area: "" } : { company: activeCompany, area: activeArea };
  return { scopeKind: "installation", ...reading };
}
