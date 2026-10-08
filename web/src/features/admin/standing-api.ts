import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, unwrap } from "@/lib/api/client";
import type { components } from "@/lib/api/schema.gen";

export type StandingApproval = components["schemas"]["StandingApproval"];
export type StandingApprovalRequest =
  components["schemas"]["StandingApprovalRequest"];

export const standingKeys = { all: ["standing-approvals"] as const };

/** The durable human grants, active and revoked, with today's uses. */
export function useStandingApprovals() {
  return useQuery({
    queryKey: standingKeys.all,
    queryFn: async () =>
      unwrap(await api.GET("/admin/standing-approvals")).items,
  });
}

export function useCreateStandingApproval() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (body: StandingApprovalRequest) =>
      unwrap(await api.POST("/admin/standing-approvals", { body })),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: standingKeys.all });
    },
  });
}

export function useRevokeStandingApproval() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (grant: string) =>
      unwrap(
        await api.DELETE("/admin/standing-approvals/{grant}", {
          params: { path: { grant } },
        }),
      ),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: standingKeys.all });
    },
  });
}
