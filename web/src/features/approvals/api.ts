import { useQuery } from "@tanstack/react-query";
import { api, unwrap } from "@/lib/api/client";

export const approvalKeys = {
  all: ["approvals"] as const,
  inbox: () => [...approvalKeys.all, "inbox"] as const,
};

export function useApprovals() {
  return useQuery({
    queryKey: approvalKeys.inbox(),
    queryFn: async () => unwrap(await api.GET("/approvals", { params: { query: {} } })),
    // The inbox is what a manager keeps open; a short interval keeps it honest
    // without needing a live stream for a list this small.
    refetchInterval: 15_000,
  });
}
