import type { UseFormReturn } from "react-hook-form";
import { useTranslation } from "react-i18next";
import {
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from "@/components/ui/form";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { useAvailableConversations } from "@/features/channels/api";
import type { ConversationValues } from "@/features/channels/conversation-form-model";

// The value a select uses for "no room at all". Radix has no empty option, and
// an empty string is what the form stores for it.
const NO_ROOM = "-";

/**
 * Where these tickets are worked.
 *
 * Offered from the conversations the bot can already reach, for the reason the
 * conversation picker is: a room it cannot post in is a room where nothing can
 * be decided, and the ticket would wait for a thread that never opens. When
 * Slack will not list them, an operator pastes the id.
 */
export function ConversationReviewField({
  form,
  channel,
}: {
  form: UseFormReturn<ConversationValues>;
  channel: string;
}) {
  const { t } = useTranslation();
  const available = useAvailableConversations(channel);
  const items = available.data?.items ?? [];
  const typeItManually =
    available.isError || (available.isSuccess && items.length === 0);

  return (
    <FormField
      control={form.control}
      name="ticketReviewIn"
      render={({ field }) => (
        <FormItem>
          <FormLabel>{t("channels.ticketReviewIn")}</FormLabel>
          {typeItManually ? (
            <FormControl>
              <Input {...field} className="font-mono" placeholder="C0123ABCDEF" />
            </FormControl>
          ) : (
            <Select
              onValueChange={(id) => field.onChange(id === NO_ROOM ? "" : id)}
              value={field.value || NO_ROOM}
              disabled={available.isLoading}
            >
              <FormControl>
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
              </FormControl>
              <SelectContent>
                <SelectItem value={NO_ROOM}>
                  {t("channels.ticketReviewInNone")}
                </SelectItem>
                {items.map((one) => (
                  <SelectItem key={one.id} value={one.id}>
                    {one.private ? "🔒 " : "#"}
                    {one.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          )}
          <FormDescription>{t("channels.ticketReviewInExplains")}</FormDescription>
          <FormMessage />
        </FormItem>
      )}
    />
  );
}
