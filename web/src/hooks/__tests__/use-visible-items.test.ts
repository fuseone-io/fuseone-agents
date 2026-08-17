import { act, renderHook } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { useVisibleItems } from "@/hooks/use-visible-items";

describe("useVisibleItems", () => {
  it("shows one local page and then extends it", () => {
    const { result } = renderHook(() => useVisibleItems([1, 2, 3, 4, 5], 2));

    expect(result.current.visible).toEqual([1, 2]);
    expect(result.current.loaded).toBe(2);
    expect(result.current.total).toBe(5);
    expect(result.current.hasMore).toBe(true);

    act(() => result.current.loadMore());

    expect(result.current.visible).toEqual([1, 2, 3, 4]);
  });

  it("resets when the filtered list changes", () => {
    const { result, rerender } = renderHook(
      ({ items }) => useVisibleItems(items, 2),
      { initialProps: { items: [1, 2, 3, 4] } },
    );

    act(() => result.current.loadMore());
    expect(result.current.visible).toEqual([1, 2, 3, 4]);

    rerender({ items: [3, 4, 5, 6] });

    expect(result.current.visible).toEqual([3, 4]);
  });
});
