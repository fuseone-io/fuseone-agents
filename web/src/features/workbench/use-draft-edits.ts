import { useState } from "react";
import { rebase, same, type Field } from "@/features/workbench/draft-operations";
import type { AgentDefinition } from "@/lib/api/client";

type Edits = { local?: AgentDefinition; touched: ReadonlySet<Field> };

/**
 * What the person has changed in the panel and not saved yet.
 *
 * A local copy of the draft's definition and the fields they touched, kept as
 * one value so every answer from the server is judged against both at once.
 * Seeded once, when the draft arrives; the panel is remounted for another
 * draft. A newer draft is taken in only through `rebase`, which keeps every
 * field still unsaved as the person left it: nothing they typed is lost to a
 * refusal, a reload, or a save that answers while they kept typing.
 */
export function useDraftEdits(server?: AgentDefinition) {
  const [edits, setEdits] = useState<Edits>({ local: server, touched: new Set() });

  if (edits.local === undefined && server !== undefined) {
    // During render rather than in an effect, as the editor's draft does:
    // the empty panel is never shown for a draft that has arrived.
    setEdits({ local: server, touched: edits.touched });
  }

  const patch = (over: Partial<AgentDefinition>) =>
    setEdits((was) => ({
      local: was.local ? { ...was.local, ...over } : was.local,
      touched: new Set([...was.touched, ...(Object.keys(over) as Field[])]),
    }));

  /**
   * Takes in a draft the server answered. `sent` is the copy a finished save
   * was computed from: what it carried is now the server's, and only fields
   * edited since then stay unsaved. Without it, every touched field stays.
   */
  const takeIn = (next: AgentDefinition, sent?: AgentDefinition) =>
    setEdits((was) => {
      const now = was.local ?? next;
      const touched = sent
        ? new Set([...was.touched].filter((f) => !same(now[f], sent[f])))
        : was.touched;
      return { local: rebase(next, now, touched), touched };
    });

  return { local: edits.local, touched: edits.touched, patch, takeIn };
}
