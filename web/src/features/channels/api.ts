import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, unwrap } from "@/lib/api/client";

export const channelKeys = { all: ["channels"] as const };

/** The connections runs report through, with the conversations inside them. */
export function useChannels() {
  return useQuery({
    queryKey: channelKeys.all,
    queryFn: async () => unwrap(await api.GET("/admin/channels")),
  });
}

export interface ChannelInput {
  name: string;
  kind: "slack";
  workspace?: string;
  /** Omitted keeps the stored one: correcting a name must not demand
   *  re-entering a secret nobody has to hand. */
  token?: string;
  enabled?: boolean;
}

export function useSaveChannel() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: async ({ name, ...body }: ChannelInput) =>
      unwrap(
        await api.PUT("/admin/channels/{name}", {
          params: { path: { name } },
          body,
        }),
      ),
    onSuccess: () =>
      void client.invalidateQueries({ queryKey: channelKeys.all }),
  });
}

export function useDeleteChannel() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: async (name: string) =>
      unwrap(
        await api.DELETE("/admin/channels/{name}", {
          params: { path: { name } },
        }),
      ),
    onSuccess: () =>
      void client.invalidateQueries({ queryKey: channelKeys.all }),
  });
}

export interface ConversationInput {
  channel: string;
  conversation: string;
  company: string;
  area?: string;
  label?: string;
  wants?: ("parked" | "failed" | "finished")[];
}

export function useSaveConversation() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: async ({ channel, conversation, ...body }: ConversationInput) =>
      unwrap(
        await api.PUT("/admin/channels/{name}/conversations/{conversation}", {
          params: { path: { name: channel, conversation } },
          body,
        }),
      ),
    onSuccess: () =>
      void client.invalidateQueries({ queryKey: channelKeys.all }),
  });
}

export function useDeleteConversation() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: async (input: { channel: string; conversation: string }) =>
      unwrap(
        await api.DELETE(
          "/admin/channels/{name}/conversations/{conversation}",
          {
            params: {
              path: { name: input.channel, conversation: input.conversation },
            },
          },
        ),
      ),
    onSuccess: () =>
      void client.invalidateQueries({ queryKey: channelKeys.all }),
  });
}

/**
 * Posts one message saying what it is.
 *
 * The alternative to this button is configuring a channel and then waiting for
 * a run to park to find out whether the bot was ever invited — and a
 * notification that silently goes nowhere is the failure the whole feature
 * exists to avoid.
 */
export function useTestConversation() {
  return useMutation({
    mutationFn: async (input: { channel: string; conversation: string }) =>
      unwrap(
        await api.POST(
          "/admin/channels/{name}/conversations/{conversation}/test",
          {
            params: {
              path: { name: input.channel, conversation: input.conversation },
            },
          },
        ),
      ),
  });
}

/**
 * The conversations this connection can be pointed at.
 *
 * Only the ones the bot is already in, so choosing from it cannot produce a
 * configuration that saves cleanly and delivers nothing. A failure here is
 * usually an app granted `chat:write` and not `channels:read` — the screen
 * shows the reason and falls back to typing an identifier rather than
 * pretending the bot is in no channels.
 */
export function useAvailableConversations(channel: string) {
  return useQuery({
    queryKey: [...channelKeys.all, "available", channel] as const,
    queryFn: async () =>
      unwrap(
        await api.GET("/admin/channels/{name}/available", {
          params: { path: { name: channel } },
        }),
      ),
    retry: false,
  });
}

export interface BindingInput {
  channel: string;
  account: string;
  principal: string;
}

/**
 * Says who a channel account is.
 *
 * The most consequential thing configured on this screen: it grants that
 * person's authority to whoever holds the account, and nothing downstream can
 * tell the difference. Explicit on purpose — no matching on email, no
 * inferring from a display name.
 */
export function useBindIdentity() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: async ({ channel, account, principal }: BindingInput) =>
      unwrap(
        await api.PUT("/admin/channels/{name}/identities/{account}", {
          params: { path: { name: channel, account } },
          body: { principal },
        }),
      ),
    onSuccess: () =>
      void client.invalidateQueries({ queryKey: channelKeys.all }),
  });
}

export function useUnbindIdentity() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: async (input: { channel: string; account: string }) =>
      unwrap(
        await api.DELETE("/admin/channels/{name}/identities/{account}", {
          params: { path: { name: input.channel, account: input.account } },
        }),
      ),
    onSuccess: () =>
      void client.invalidateQueries({ queryKey: channelKeys.all }),
  });
}
