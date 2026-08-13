import { MessageSquare } from "lucide-react";
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
import { IntegrationsSection } from "@/features/integrations/integrations-section";
import type { components } from "@/lib/api/schema.gen";

type Channel = components["schemas"]["Channel"];

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
  const [addingTo, setAddingTo] = useState<string | null>(null);

  const channels = data?.items ?? [];

  if (isLoading) return <LoadingRows rows={3} />;
  if (error) return <ErrorState error={error} onRetry={() => void refetch()} />;

  return (
    <>
      <IntegrationsSection
        title={t("channels.channels")}
        onAdd={() => setEditing(null)}
        empty={
          channels.length === 0 && (
            <EmptyState
              icon={<MessageSquare className="size-6" />}
              title={t("channels.none")}
              hint={t("channels.noneHint")}
            />
          )
        }
      >
        {channels.map((channel) => (
          <ChannelCard
            key={channel.name}
            channel={channel}
            onEdit={() => setEditing(channel)}
            onAddConversation={() => setAddingTo(channel.name)}
          />
        ))}
      </IntegrationsSection>

      {editing !== undefined && (
        <ChannelForm channel={editing} onClose={() => setEditing(undefined)} />
      )}
      {addingTo && (
        <ConversationForm
          channel={addingTo}
          onClose={() => setAddingTo(null)}
        />
      )}
    </>
  );
}
