import { useMemo, useState } from "react";

interface VisibleState<T> {
  items: T[];
  pageSize: number;
  count: number;
}

export function useVisibleItems<T>(items: T[], pageSize: number) {
  const [state, setState] = useState<VisibleState<T>>({
    items,
    pageSize,
    count: pageSize,
  });

  const count =
    sameItems(state.items, items) && state.pageSize === pageSize
      ? state.count
      : pageSize;
  const visible = useMemo(() => items.slice(0, count), [items, count]);
  const loaded = Math.min(count, items.length);

  return {
    visible,
    loaded,
    total: items.length,
    hasMore: loaded < items.length,
    loadMore: () =>
      setState((current) => {
        const count =
          sameItems(current.items, items) && current.pageSize === pageSize
            ? current.count
            : pageSize;
        return {
          items,
          pageSize,
          count: Math.min(count + pageSize, items.length),
        };
      }),
  };
}

function sameItems<T>(left: T[], right: T[]) {
  if (left.length !== right.length) return false;
  return left.every((item, index) => Object.is(item, right[index]));
}
