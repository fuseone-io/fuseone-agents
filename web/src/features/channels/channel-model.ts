import type { components } from "@/lib/api/schema.gen";

export type Channel = components["schemas"]["Channel"];
export type Conversation = components["schemas"]["ChannelConversation"];
export type ChannelView = "all" | "attention" | "approvals";

export function channelNeedsAttention(channel: Channel) {
  if (!channel.enabled || !channel.hasCredential) return true;
  if (channel.deliveryMode === "socket") return !channel.hasAppToken;
  return !channel.hasSigning;
}

export function channelHealth(channel: Channel): {
  key: string;
  tone: "ok" | "warn";
} {
  if (!channel.enabled) return { key: "disabled", tone: "warn" };
  if (!channel.hasCredential) return { key: "noCredential", tone: "warn" };
  if (channel.deliveryMode === "socket" && !channel.hasAppToken) {
    return { key: "noAppToken", tone: "warn" };
  }
  if (channel.deliveryMode !== "socket" && !channel.hasSigning) {
    return { key: "outboundOnly", tone: "warn" };
  }
  return { key: "answering", tone: "ok" };
}

export function filterConversations(
  conversations: Conversation[],
  query: string,
  view: ChannelView,
  channelAttention: boolean,
) {
  const q = query.trim().toLowerCase();
  return conversations.filter((conversation) => {
    if (view === "approvals" && !(conversation.wants ?? []).includes("parked")) {
      return false;
    }
    if (view === "attention" && !channelAttention && conversation.enabled) {
      return false;
    }
    if (!q) return true;
    const scope = conversation.scope.area
      ? `${conversation.scope.company}/${conversation.scope.area}`
      : conversation.scope.company;
    return [
      conversation.id,
      conversation.label ?? "",
      scope,
      conversation.mode ?? "mentions",
      ...(conversation.wants ?? []),
    ]
      .join(" ")
      .toLowerCase()
      .includes(q);
  });
}

export function visibleChannels(
  channels: Channel[],
  query: string,
  view: ChannelView,
) {
  const q = query.trim();
  return channels.filter((channel) => {
    if (view === "all" && q === "") return true;
    const attention = channelNeedsAttention(channel);
    const matches = filterConversations(
      channel.conversations,
      query,
      view,
      attention,
    );
    return matches.length > 0 || (view === "attention" && attention);
  });
}
