import { Plus } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { ConversationRow } from "@/features/channels/conversation-row";
import type { Conversation } from "@/features/channels/channel-model";

export function ConversationList({
  channel,
  conversations,
  total,
  allTotal,
  hidden,
  onExpand,
  onAdd,
  onEdit,
}: {
  channel: string;
  conversations: Conversation[];
  total: number;
  allTotal: number;
  hidden: number;
  onExpand: () => void;
  onAdd: () => void;
  onEdit: (conversation: Conversation) => void;
}) {
  const { t } = useTranslation();

  return (
    <div>
      <Table className="min-w-[840px] table-fixed">
        <TableHeader>
          <TableRow className="bg-muted/50 hover:bg-muted/50">
            <TableHead className="w-[28%]">{t("channels.conversation")}</TableHead>
            <TableHead className="w-[30%]">{t("channels.sends")}</TableHead>
            <TableHead className="w-[17%]">{t("channels.listensTo")}</TableHead>
            <TableHead className="w-[15%]">{t("channels.lastDelivery")}</TableHead>
            <TableHead className="w-[10%] text-right">
              {t("common.actions")}
            </TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {conversations.length === 0 ? (
            <TableRow>
              <TableCell
                colSpan={5}
                className="h-20 text-center text-xs text-muted-foreground"
              >
                {allTotal === 0
                  ? t("channels.noConversation")
                  : t("channels.noConversationMatches")}
              </TableCell>
            </TableRow>
          ) : (
            conversations.map((conversation) => (
              <ConversationRow
                key={conversation.id}
                channel={channel}
                conversation={conversation}
                onEdit={() => onEdit(conversation)}
              />
            ))
          )}
        </TableBody>
      </Table>
      <div className="flex flex-wrap items-center gap-2 border-t px-4 py-2">
        <Button
          variant="outline"
          size="sm"
          className="border-dashed"
          onClick={onAdd}
        >
          <Plus className="size-4" aria-hidden />
          {t("channels.addConversation")}
        </Button>
        {hidden > 0 && (
          <Button variant="outline" size="sm" onClick={onExpand}>
            {t("channels.showMoreConversations", { count: hidden })}
          </Button>
        )}
        <span className="ml-auto text-xs text-muted-foreground">
          {total === allTotal
            ? t("channels.conversationCount", { count: allTotal })
            : t("channels.showingConversations", {
                shown: conversations.length,
                total,
              })}
        </span>
      </div>
    </div>
  );
}
