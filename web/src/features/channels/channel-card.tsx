import { Send, Trash2 } from "lucide-react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Separator } from "@/components/ui/separator";
import { IdentityRows } from "@/features/channels/identity-rows";
import {
  useDeleteConversation,
  useTestConversation,
} from "@/features/channels/api";
import { problemMessage } from "@/lib/api/problem-message";
import type { components } from "@/lib/api/schema.gen";

type Channel = components["schemas"]["Channel"];

interface ChannelCardProps {
  channel: Channel;
  onEdit: () => void;
  onAddConversation: () => void;
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
  onEdit,
  onAddConversation,
}: ChannelCardProps) {
  const { t } = useTranslation();
  const inboundReady =
    channel.deliveryMode === "socket" ? channel.hasAppToken : channel.hasSigning;

  return (
    <Card className="gap-0 p-0">
      <div className="flex items-center gap-3 p-4">
        <div className="min-w-0 flex-1">
          <p className="truncate text-sm font-medium">{channel.name}</p>
          <p className="truncate text-xs text-muted-foreground">
            {channel.workspace || channel.kind}
          </p>
        </div>
        {!channel.hasCredential && (
          <Badge variant="outline" className="text-warning">
            {t("channels.noCredential")}
          </Badge>
        )}
        {channel.deliveryMode === "socket" && (
          <Badge variant="outline">{t("channels.deliverySocket")}</Badge>
        )}
        {channel.deliveryMode === "socket" && !channel.hasAppToken && (
          <Badge variant="outline" className="text-warning">
            {t("channels.noAppToken")}
          </Badge>
        )}
        {!channel.enabled && (
          <Badge variant="outline">{t("common.disabled")}</Badge>
        )}
        <Button variant="outline" size="sm" onClick={onEdit}>
          {t("common.edit")}
        </Button>
      </div>

      <Separator />

      <div className="flex flex-col gap-1 p-2">
        {channel.conversations.length === 0 ? (
          <p className="px-2 py-3 text-xs text-muted-foreground">
            {t("channels.noConversation")}
          </p>
        ) : (
          channel.conversations.map((c) => (
            <ConversationRow
              key={c.id}
              channel={channel.name}
              conversation={c}
            />
          ))
        )}
        <Button
          variant="ghost"
          size="sm"
          className="justify-start"
          onClick={onAddConversation}
        >
          {t("channels.addConversation")}
        </Button>
      </div>

      {/* Only where an answer could arrive. Binding accounts on a channel that
          cannot verify what comes back would be configuring authority for a
          door that is shut. */}
      {inboundReady && (
        <>
          <Separator />
          <IdentityRows
            channel={channel.name}
            identities={channel.identities ?? []}
            seenAccounts={channel.seenAccounts ?? []}
          />
        </>
      )}
    </Card>
  );
}

function ConversationRow({
  channel,
  conversation,
}: {
  channel: string;
  conversation: components["schemas"]["ChannelConversation"];
}) {
  const { t } = useTranslation();
  const test = useTestConversation();
  const remove = useDeleteConversation();
  const scope = conversation.scope.area
    ? `${conversation.scope.company}/${conversation.scope.area}`
    : conversation.scope.company;
  const mode =
    conversation.mode === "watch"
      ? `${t("channels.modeWatch")} · ${conversation.agent ?? "—"}`
      : t("channels.modeMentions");

  return (
    <div className="flex items-center gap-2 rounded-md px-2 py-1.5 hover:bg-muted/50">
      <div className="min-w-0 flex-1">
        <p className="truncate text-sm">
          {conversation.label || conversation.id}
        </p>
        <p className="truncate font-mono text-2xs tabular-nums text-muted-foreground">
          {scope} · {(conversation.wants ?? ["parked", "failed"]).join(", ")}
        </p>
        <p className="truncate text-2xs text-muted-foreground">{mode}</p>
      </div>
      <Button
        variant="ghost"
        size="icon"
        aria-label={t("channels.sendTest")}
        disabled={test.isPending}
        onClick={() =>
          test.mutate(
            { channel, conversation: conversation.id },
            {
              onSuccess: () => toast.success(t("channels.testDelivered")),
              onError: (error) => toast.error(problemMessage(error, t)),
            },
          )
        }
      >
        <Send className="size-4" />
      </Button>
      <Button
        variant="ghost"
        size="icon"
        aria-label={t("common.remove")}
        onClick={() =>
          remove.mutate({ channel, conversation: conversation.id })
        }
      >
        <Trash2 className="size-4" />
      </Button>
    </div>
  );
}
