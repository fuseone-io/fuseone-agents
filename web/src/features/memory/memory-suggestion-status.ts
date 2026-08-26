import type { MemorySuggestionStatus } from "@/features/memory/api";

export const MEMORY_SUGGESTION_STATUS_LABELS: Record<MemorySuggestionStatus, string> = {
  accepted: "memory.suggestionStatus.accepted",
  auto_confirmed: "memory.suggestionStatus.autoConfirmed",
  dismissed: "memory.suggestionStatus.dismissed",
  pending: "memory.suggestionStatus.pending",
  source_erased: "memory.suggestionStatus.sourceErased",
};

export function memorySuggestionStatusClass(status: MemorySuggestionStatus): string {
  if (status === "pending") return "bg-warning-surface text-warning";
  if (status === "accepted" || status === "auto_confirmed") {
    return "bg-success-surface text-success";
  }
  return "bg-muted text-muted-foreground";
}
