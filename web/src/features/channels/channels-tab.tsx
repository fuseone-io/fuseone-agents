import { MessageSquare, Search } from "lucide-react";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import {
  EmptyState,
  ErrorState,
  LoadingRows,
} from "@/components/shared/states";
import { useChannels } from "@/features/channels/api";
import { ChannelCard } from "@/features/channels/channel-card";
import { ChannelForm } from "@/features/channels/channel-form";
import { ConversationForm } from "@/features/channels/conversation-form";
import { ChannelsToolbar } from "@/features/channels/channels-toolbar";
import {
  type ChannelView,
  visibleChannels,
} from "@/features/channels/channel-model";
import type { components } from "@/lib/api/schema.gen";

type Channel = components["schemas"]["Channel"];
type Conversation = components["schemas"]["ChannelConversation"];
type ConversationDialog = {
  channel: string;
  conversation?: Conversation;
};

/**
 * Where runs report.
 *
 * Beside the tool servers and the model providers because it is the same job —
 * what this installation is connected to — and unlike either of those, nothing
 * here grants an agent any new ability. A channel is somewhere runs speak, and
 * nothing it says comes back.
 */
export function ChannelsTab() {
  const { t } = useTranslation();
  const { data, isLoading, error, refetch } = useChannels();
  const [editing, setEditing] = useState<Channel | null | undefined>(undefined);
  const [conversationDialog, setConversationDialog] =
    useState<ConversationDialog | null>(null);
  const [query, setQuery] = useState("");
  const [view, setView] = useState<ChannelView>("all");

  const channels = data?.items ?? [];
  const shownChannels = visibleChannels(channels, query, view);

  if (isLoading) return <LoadingRows rows={3} />;
  if (error) return <ErrorState error={error} onRetry={() => void refetch()} />;

  return (
    <>
      <section className="flex flex-col gap-3">
        <ChannelsToolbar
          query={query}
          view={view}
          onQuery={setQuery}
          onView={setView}
          onAdd={() => setEditing(null)}
        />

        {channels.length === 0 ? (
          <EmptyState
            icon={<MessageSquare className="size-6" />}
            title={t("channels.none")}
            hint={t("channels.noneHint")}
          />
        ) : shownChannels.length === 0 ? (
          <EmptyState
            icon={<Search className="size-6" />}
            title={t("channels.noMatches")}
            hint={t("channels.noMatchesHint")}
          />
        ) : (
          shownChannels.map((channel) => (
            <ChannelCard
              key={channel.name}
              channel={channel}
              query={query}
              view={view}
              onEdit={() => setEditing(channel)}
              onAddConversation={() =>
                setConversationDialog({ channel: channel.name })
              }
              onEditConversation={(conversation) =>
                setConversationDialog({ channel: channel.name, conversation })
              }
            />
          ))
        )}
      </section>

      {editing !== undefined && (
        <ChannelForm
          channel={editing}
          kinds={data?.kinds ?? []}
          onClose={() => setEditing(undefined)}
        />
      )}
      {conversationDialog && (
        <ConversationForm
          key={`${conversationDialog.channel}/${
            conversationDialog.conversation?.id ?? "new"
          }`}
          channel={conversationDialog.channel}
          conversation={conversationDialog.conversation}
          onClose={() => setConversationDialog(null)}
        />
      )}
    </>
  );
}
