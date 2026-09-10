import { RefreshCw } from "lucide-react";
import type { UseFormReturn } from "react-hook-form";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
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
import { problemMessage } from "@/lib/api/problem-message";
import type { ConversationValues } from "@/features/channels/conversation-form-model";

/**
 * Which conversation this is.
 *
 * Listing is a convenience, not authority. When Slack refuses the list or
 * returns none, the screen says so and lets an operator paste the stable
 * channel id rather than leaving behind a picker with no choices.
 *
 * The id of a conversation that already exists cannot change: it is the name
 * the row is stored under, so editing it would silently create a second
 * conversation and leave the first one receiving.
 */
export function ConversationIdField({
  form,
  channel,
  existing,
}: {
  form: UseFormReturn<ConversationValues>;
  channel: string;
  existing: boolean;
}) {
  const { t } = useTranslation();
  const available = useAvailableConversations(channel);
  const items = available.data?.items ?? [];
  const typeItManually =
    available.isError || (available.isSuccess && items.length === 0);

  return (
    <FormField
      control={form.control}
      name="conversation"
      render={({ field }) => (
        <FormItem>
          <FormLabel>{t("channels.conversation")}</FormLabel>
          {existing ? (
            <>
              <FormControl>
                <Input {...field} disabled className="font-mono" />
              </FormControl>
              <FormDescription>
                {t("channels.conversationIdCannotChange")}
              </FormDescription>
            </>
          ) : typeItManually ? (
            <>
              <FormControl>
                <Input {...field} placeholder="C0123ABCDEF" />
              </FormControl>
              <FormDescription
                className={available.isError ? "text-warning" : ""}
              >
                {available.isError
                  ? problemMessage(available.error, t)
                  : t("channels.noAvailableConversations")}
              </FormDescription>
              {!available.isError && (
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  className="w-fit"
                  disabled={available.isFetching}
                  onClick={() => void available.refetch()}
                >
                  <RefreshCw className="size-3.5" aria-hidden />
                  {t("common.retry")}
                </Button>
              )}
            </>
          ) : (
            <Select
              onValueChange={(id) => {
                field.onChange(id);
                const picked = items.find((c) => c.id === id);
                if (picked) form.setValue("label", `#${picked.name}`);
              }}
              value={field.value ?? ""}
              disabled={available.isLoading}
            >
              <FormControl>
                <SelectTrigger>
                  <SelectValue
                    placeholder={
                      available.isLoading
                        ? t("common.loadingMore")
                        : t("channels.pickConversation")
                    }
                  />
                </SelectTrigger>
              </FormControl>
              <SelectContent>
                {items.map((c) => (
                  <SelectItem key={c.id} value={c.id}>
                    {c.private ? "🔒 " : "#"}
                    {c.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          )}
          <FormDescription>{t("channels.onlyWhereInvited")}</FormDescription>
          <FormMessage />
        </FormItem>
      )}
    />
  );
}
