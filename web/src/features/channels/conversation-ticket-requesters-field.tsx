import type { UseFormReturn } from "react-hook-form";
import { useTranslation } from "react-i18next";
import { Checkbox } from "@/components/ui/checkbox";
import { FormField, FormItem, FormMessage } from "@/components/ui/form";
import type { ConversationValues } from "@/features/channels/conversation-form-model";

/**
 * Whether somebody nobody linked may open a ticket as their Slack account.
 *
 * Asked rather than assumed: refusing the unbound is how every room behaved
 * before, and a requester known only by Slack is evidence, never authority.
 */
export function ConversationTicketRequestersField({
  form,
}: {
  form: UseFormReturn<ConversationValues>;
}) {
  const { t } = useTranslation();

  return (
    <FormField
      control={form.control}
      name="ticketSlackRequesters"
      render={({ field }) => (
        <FormItem>
          <label className="flex items-start gap-2 text-sm">
            <Checkbox
              checked={field.value}
              onCheckedChange={(on) => field.onChange(Boolean(on))}
            />
            <span className="grid gap-1">
              <span className="font-medium">{t("channels.ticketSlackRequesters")}</span>
              <span className="text-muted-foreground">
                {t("channels.ticketSlackRequestersExplains")}
              </span>
            </span>
          </label>
          <FormMessage />
        </FormItem>
      )}
    />
  );
}
