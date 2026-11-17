import { useState } from "react";
import { rebase, type Field } from "@/features/workbench/draft-operations";
import type { AgentDefinition } from "@/lib/api/client";

/**
 * What the person has changed in the panel and not saved yet.
 *
 * A local copy of the draft's definition and the fields they touched. Seeded
 * once, when the draft arrives; afterwards a newer draft from the server is
 * taken in only through `rebase`, which keeps every field they touched as
 * they left it. Nothing they typed is lost to a refusal or a reload.
 */
export function useDraftEdits(server?: AgentDefinition) {
  const [local, setLocal] = useState<AgentDefinition | undefined>(server);
  const [touched, setTouched] = useState<ReadonlySet<Field>>(new Set());

  if (local === undefined && server !== undefined) {
    // During render rather than in an effect, as the editor's draft does:
    // the empty panel is never shown for a draft that has arrived.
    setLocal(server);
  }

  const patch = (over: Partial<AgentDefinition>) => {
    setLocal((was) => (was ? { ...was, ...over } : was));
    setTouched((was) => new Set([...was, ...(Object.keys(over) as Field[])]));
  };

  /** Takes in a draft the server answered, keeping what is still unsaved. */
  const takeIn = (next: AgentDefinition, saved: boolean) => {
    if (saved) {
      setLocal(next);
      setTouched(new Set());
      return;
    }
    setLocal((was) => rebase(next, was ?? next, touched));
  };

  return { local, touched, patch, takeIn };
}
