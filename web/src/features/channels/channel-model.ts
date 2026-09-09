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

/*
scopeText is where a conversation was configured, as one string.

Shared rather than written out at each call site, because the listing, the
search and the form each need the same answer and three copies drift: the day
"*" started meaning the whole installation, a copy nobody remembered would go
on printing a company called "*".

It stays the stored value. Turning it into a name needs the translation
dictionary, and the search has to match what is stored anyway.
*/
export function scopeText(scope: Conversation["scope"]) {
  return scope.area ? `${scope.company}/${scope.area}` : scope.company;
}

export function filterConversations(
  conversations: Conversation[],
  query: string,
  view: ChannelView,
  channelAttention: boolean,
) {
  const q = query.trim().toLowerCase();
  return conversations.filter((conversation) => {
    if (
      view === "approvals" &&
      !(conversation.wants ?? []).includes("parked")
    ) {
      return false;
    }
    if (view === "attention" && !channelAttention && conversation.enabled) {
      return false;
    }
    if (!q) return true;
    return [
      conversation.id,
      conversation.label ?? "",
      scopeText(conversation.scope),
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
