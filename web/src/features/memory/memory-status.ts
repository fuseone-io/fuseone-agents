import type { MemoryStatus } from "@/features/memory/api";

export const MEMORY_STATUS_LABELS: Record<MemoryStatus, string> = {
  active: "memory.status.active",
  disabled: "memory.status.disabled",
  expired: "memory.status.expired",
  source_erased: "memory.status.sourceErased",
};

export function memoryStatusClass(status: MemoryStatus): string {
  if (status === "active") return "bg-success-surface text-success";
  if (status === "disabled") return "bg-muted text-muted-foreground";
  return "bg-warning-surface text-warning";
}
