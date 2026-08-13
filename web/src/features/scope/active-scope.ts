import { create } from "zustand";
import { createJSONStorage, persist } from "zustand/middleware";
import type { MeGrant } from "@/features/session/api";
import { durableOrMemory } from "@/lib/durable-storage";

/**
 * Which company and area the console is currently reading.
 *
 * An empty company means no choice has been made, and no choice means the API
 * answers with everything the caller reaches — the same thing it did before
 * this existed. That is the honest default: narrowing a screen on somebody's
 * behalf, without them having asked, makes an empty page look like an outage.
 *
 * An empty area inside a chosen company means the company itself, which is the
 * same convention grants and ceilings already use.
 */
export interface Scope {
  company: string;
  area: string;
}

interface ActiveScope extends Scope {
  choose: (scope: Scope) => void;
  /**
   * Drops a stored scope the caller no longer reaches. Without it, losing a
   * grant — or opening the console as somebody else on a shared machine —
   * filters every screen to a scope the server will refuse, and the console
   * shows empty tables rather than saying why.
   */
  reconcile: (grants: MeGrant[]) => void;
}

export const useActiveScope = create<ActiveScope>()(
  persist(
    (set, get) => ({
      company: "",
      area: "",
      choose: ({ company, area }) => set({ company, area }),
      reconcile: (grants) => {
        const { company, area } = get();
        if (company === "") return;
        if (!reaches(grants, company, area)) set({ company: "", area: "" });
      },
    }),
    {
      name: "fuseone.scope",
      partialize: ({ company, area }) => ({ company, area }),
      storage: createJSONStorage(durableOrMemory("fuseone.scope")),
    },
  ),
);

/** A grant over a whole company reaches every area in it. */
function reaches(grants: MeGrant[], company: string, area: string): boolean {
  return grants.some(
    (g) => g.company === company && (g.area === "" || g.area === area),
  );
}

/**
 * The query parameters for a scope. Absent rather than empty: `company=` is a
 * filter for a company named "", and the endpoints read a present parameter as
 * a scope to check.
 */
export function scopeParamsOf({ company, area }: Scope): {
  company?: string;
  area?: string;
} {
  if (company === "") return {};
  return area === "" ? { company } : { company, area };
}

/** The fragment that makes a query key belong to one context.
 *  Without it, switching context serves the previous one from cache. */
export function scopeKeyOf({ company, area }: Scope): string {
  return company === "" ? "*" : `${company}/${area}`;
}
