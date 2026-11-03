import { useState } from "react";
import { useScopeFilter } from "@/features/scope/use-scope-filter";
import { useMeasureChoices } from "@/features/work-items/use-measure-choices";
import type { MeasureView } from "@/features/work-items/work-item-measures-filters";

const DEFAULT_WEEKS = 8;

/**
 * The weekly panel's view: how many weeks, and the filters within the scope.
 *
 * The filters belong to the scope they were chosen in. They are stored with
 * that scope's key and read back only while it is still the active one, so
 * switching scope clears them on the very render that switches — there is no
 * frame in which the old area is asked of the new scope. The weeks carry over:
 * they say nothing about where.
 */
export function useMeasureView() {
  const scope = useScopeFilter();
  const [stored, setStored] = useState<{ key: string; view: MeasureView }>({
    key: scope.key,
    view: { weeks: DEFAULT_WEEKS },
  });
  const view =
    stored.key === scope.key ? stored.view : { weeks: stored.view.weeks };
  const choices = useMeasureChoices(view);
  return {
    weeks: view.weeks,
    choices,
    setView: (next: MeasureView) => setStored({ key: scope.key, view: next }),
  };
}
