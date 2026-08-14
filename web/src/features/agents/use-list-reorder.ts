import { useRef, useState } from "react";

/**
 * Dragging one row of a list onto another.
 *
 * Held in a ref rather than in state, because a drag that re-rendered on
 * every hover would reorder the list under the pointer and drop the row
 * somewhere nobody aimed at. The list moves once, when the drag ends.
 */
export function useListReorder(onMove: (from: number, to: number) => void) {
  const from = useRef<number | undefined>(undefined);
  const [over, setOver] = useState<number | undefined>(undefined);

  return {
    over,
    onStart: (at: number) => {
      from.current = at;
    },
    onOver: (at: number) => setOver(at),
    onDrop: () => {
      const start = from.current;
      if (start !== undefined && over !== undefined && start !== over) {
        onMove(start, over);
      }
      from.current = undefined;
      setOver(undefined);
    },
  };
}
