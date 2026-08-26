import type { MemoryStatusFilter } from "@/features/memory/api";

export type MemoryView = "active" | "disabled" | "suggested" | "all";

export function memoryStatusForView(view: MemoryView): MemoryStatusFilter {
  if (view === "active" || view === "disabled") return view;
  return "all";
}
