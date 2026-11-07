import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { serviceAccountKeys } from "@/features/admin/service-account-keys";
import { peopleKeys } from "@/features/admin/people-api";
import { api, unwrap } from "@/lib/api/client";
import type { components } from "@/lib/api/schema.gen";

export type ServiceAccount = components["schemas"]["ServiceAccount"];
export type ServiceToken = components["schemas"]["ServiceToken"];

export function useServiceAccounts() {
  return useQuery({
    queryKey: serviceAccountKeys.all,
    queryFn: async () => unwrap(await api.GET("/admin/service-accounts")),
    retry: false,
  });
}

export function useServiceTokens(account: string) {
  return useQuery({
    queryKey: serviceAccountKeys.tokens(account),
    queryFn: async () =>
      unwrap(
        await api.GET("/admin/service-accounts/{accountId}/tokens", {
          params: { path: { accountId: account } },
        }),
      ),
    retry: false,
  });
}

/** The directory lists machines too, so both have to hear about a change. */
function useAccountsChanged() {
  const queryClient = useQueryClient();
  return () => {
    void queryClient.invalidateQueries({ queryKey: serviceAccountKeys.all });
    void queryClient.invalidateQueries({ queryKey: peopleKeys.all });
  };
}

export function useCreateServiceAccount() {
  const changed = useAccountsChanged();
  return useMutation({
    mutationFn: async (body: { name: string; description?: string }) =>
      unwrap(await api.POST("/admin/service-accounts", { body })),
    onSuccess: changed,
  });
}

export function useSetServiceAccountDisabled() {
  const changed = useAccountsChanged();
  return useMutation({
    mutationFn: async (input: { account: string; disabled: boolean }) =>
      unwrap(
        await api.PUT("/admin/service-accounts/{accountId}/disabled", {
          params: { path: { accountId: input.account } },
          body: { disabled: input.disabled },
        }),
      ),
    onSuccess: changed,
  });
}

/**
 * Issues a token and hands its secret to `reveal`, and to nothing else.
 *
 * What a mutation returns stays in the mutation cache, where anything reading
 * it finds it for as long as the tab lives. So the secret never becomes the
 * result: the one copy is whatever the caller does with `reveal`, and the
 * caller drops it when the dialog closes.
 */
export function useIssueServiceToken(reveal: (secret: string) => void) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (input: {
      account: string;
      name: string;
      expiresAt: string;
    }) => {
      const { secret, ...token } = unwrap(
        await api.POST("/admin/service-accounts/{accountId}/tokens", {
          params: { path: { accountId: input.account } },
          body: { name: input.name, expiresAt: input.expiresAt },
        }),
      );
      reveal(secret);
      return token;
    },
    onSuccess: (_token, input) =>
      void queryClient.invalidateQueries({
        queryKey: serviceAccountKeys.tokens(input.account),
      }),
  });
}

export function useRevokeServiceToken() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (input: { account: string; token: string }) =>
      unwrap(
        await api.DELETE("/admin/service-accounts/{accountId}/tokens/{tokenId}", {
          params: { path: { accountId: input.account, tokenId: input.token } },
        }),
      ),
    onSuccess: (_nothing, input) =>
      void queryClient.invalidateQueries({
        queryKey: serviceAccountKeys.tokens(input.account),
      }),
  });
}
