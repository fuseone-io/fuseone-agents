import { useTranslation } from "react-i18next";
import { useState } from "react";
import {
  ChevronDown,
  ChevronRight,
  MessageSquare,
  Settings2,
} from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Separator } from "@/components/ui/separator";
import { ConversationList } from "@/features/channels/conversation-list";
import { ChannelIdentityStrip } from "@/features/channels/channel-identity-strip";
import {
  channelHealth,
  channelNeedsAttention,
  filterConversations,
  type Channel,
  type ChannelView,
  type Conversation,
} from "@/features/channels/channel-model";
import { cn } from "@/lib/utils";

interface ChannelCardProps {
  channel: Channel;
  query: string;
  view: ChannelView;
  onEdit: () => void;
  onAddConversation: () => void;
  onEditConversation: (conversation: Conversation) => void;
}

/**
 * One connection, and everywhere it speaks.
 *
 * The conversations are here rather than on a screen of their own because a
 * conversation without its connection is meaningless — and because the scope
 * each one carries is the governing fact, so it has to be readable beside the
 * place it points at.
 */
export function ChannelCard({
  channel,
  query,
  view,
  onEdit,
  onAddConversation,
  onEditConversation,
}: ChannelCardProps) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(true);
  const [expanded, setExpanded] = useState(false);
  const inboundReady =
    channel.deliveryMode === "socket" ? channel.hasAppToken : channel.hasSigning;
  const identities = channel.identities ?? [];
  const attention = channelNeedsAttention(channel);
  const rows = filterConversations(channel.conversations, query, view, attention);
  const visibleRows = expanded || query.trim() !== "" ? rows : rows.slice(0, 6);
  const hidden = rows.length - visibleRows.length;
  const health = channelHealth(channel);

  return (
    <Card className="gap-0 overflow-hidden p-0">
      <div className="flex flex-wrap items-center gap-3 border-b p-3">
        <Button
          variant="ghost"
          size="icon"
          className="size-7 shrink-0"
          aria-label={open ? t("channels.collapse") : t("channels.expand")}
          onClick={() => setOpen((current) => !current)}
        >
          {open ? (
            <ChevronDown className="size-4" aria-hidden />
          ) : (
            <ChevronRight className="size-4" aria-hidden />
          )}
        </Button>
        <span className="grid size-8 shrink-0 place-items-center rounded-md border bg-muted text-muted-foreground">
          <MessageSquare className="size-4" aria-hidden />
        </span>
        <div className="min-w-0 flex-1">
          <div className="flex min-w-0 flex-wrap items-center gap-2">
            <p className="truncate text-sm font-medium">{channel.name}</p>
            <Badge variant="outline" className="shrink-0">
              {channel.deliveryMode === "socket"
                ? t("channels.deliverySocket")
                : t("channels.deliveryHttp")}
            </Badge>
          </div>
          <p className="truncate text-xs text-muted-foreground">
            {t("channels.workspaceMeta", {
              workspace: channel.workspace || channel.kind,
              count: channel.conversations.length,
            })}
          </p>
        </div>
        <span
          className={cn(
            "inline-flex items-center gap-1.5 whitespace-nowrap text-xs",
            health.tone === "ok" ? "text-success" : "text-warning",
          )}
        >
          <span className="size-1.5 rounded-full bg-current" aria-hidden />
          {t(`channels.health.${health.key}`)}
        </span>
        <Button variant="outline" size="sm" onClick={onEdit}>
          <Settings2 className="size-4" aria-hidden />
          {t("common.edit")}
        </Button>
      </div>

      {/* Only where an answer could arrive. Binding accounts on a channel that
          cannot verify what comes back would be configuring authority for a
          door that is shut. */}
      {open && inboundReady && (
        <>
          <ChannelIdentityStrip
            channel={channel.name}
            identities={identities}
            seenAccounts={channel.seenAccounts ?? []}
          />
          <Separator />
        </>
      )}

      {open && (
        <ConversationList
          channel={channel.name}
          conversations={visibleRows}
          total={rows.length}
          allTotal={channel.conversations.length}
          hidden={hidden}
          onExpand={() => setExpanded(true)}
          onAdd={onAddConversation}
          onEdit={onEditConversation}
        />
      )}
    </Card>
  );
}
