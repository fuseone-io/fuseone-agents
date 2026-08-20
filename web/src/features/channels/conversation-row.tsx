import { AtSign, Hash, Pencil, Send, Trash2 } from "lucide-react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { TableCell, TableRow } from "@/components/ui/table";
import {
  useDeleteConversation,
  useTestConversation,
} from "@/features/channels/api";
import type { Conversation } from "@/features/channels/channel-model";
import { problemMessage } from "@/lib/api/problem-message";

export function ConversationRow({
  channel,
  conversation,
  onEdit,
}: {
  channel: string;
  conversation: Conversation;
  onEdit: () => void;
}) {
  const { t } = useTranslation();
  const test = useTestConversation();
  const remove = useDeleteConversation();
  const scope = conversation.scope.area
    ? `${conversation.scope.company}/${conversation.scope.area}`
    : conversation.scope.company;
  const threadContext = conversation.threadContext
    ? ` · ${t("channels.threadContextShort")}`
    : "";
  const mode =
    conversation.mode === "watch"
      ? `${t("channels.modeWatch")} · ${conversation.agent ?? "-"}`
      : conversation.mode === "both"
        ? `${t("channels.modeBoth")} · ${conversation.agent ?? "-"}${threadContext}`
        : `${t("channels.modeMentions")}${threadContext}`;

  return (
    <TableRow>
      <TableCell>
        <div className="flex min-w-0 items-center gap-2">
          {(conversation.label || conversation.id).startsWith("@") ? (
            <AtSign className="size-4 shrink-0 text-muted-foreground" aria-hidden />
          ) : (
            <Hash className="size-4 shrink-0 text-muted-foreground" aria-hidden />
          )}
          <div className="min-w-0">
            <p className="truncate font-mono text-sm">
              {conversation.label || conversation.id}
            </p>
            <p className="truncate text-2xs text-muted-foreground">{scope}</p>
          </div>
        </div>
      </TableCell>
      <TableCell>
        <EventBadges wants={conversation.wants ?? ["parked", "failed"]} />
      </TableCell>
      <TableCell className="truncate text-xs">{mode}</TableCell>
      <TableCell className="truncate text-xs text-muted-foreground">
        {conversation.enabled ? t("channels.notTracked") : t("common.disabled")}
      </TableCell>
      <TableCell>
        <div className="flex justify-end gap-1">
          <Button
            variant="ghost"
            size="icon"
            aria-label={t("channels.editConversation")}
            onClick={onEdit}
          >
            <Pencil className="size-4" />
          </Button>
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
      </TableCell>
    </TableRow>
  );
}

function EventBadges({ wants }: { wants: string[] }) {
  const { t } = useTranslation();
  if (wants.length === 0) {
    return (
      <span className="text-xs text-muted-foreground">
        {t("channels.nothingRouted")}
      </span>
    );
  }
  return (
    <div className="flex min-w-0 flex-wrap gap-1">
      {wants.slice(0, 3).map((want) => (
        <Badge key={want} variant={eventVariant(want)} className="text-2xs">
          {t(`channels.event.${want}`)}
        </Badge>
      ))}
      {wants.length > 3 && (
        <span className="text-xs text-muted-foreground">
          +{wants.length - 3}
        </span>
      )}
    </div>
  );
}

function eventVariant(want: string): "secondary" | "destructive" | "outline" {
  if (want === "failed") return "destructive";
  if (want === "finished") return "outline";
  return "secondary";
}
