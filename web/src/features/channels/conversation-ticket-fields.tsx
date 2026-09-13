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
import { Textarea } from "@/components/ui/textarea";
import type { ConversationValues } from "@/features/channels/conversation-form-model";
import { ConversationRunAsField } from "@/features/channels/conversation-run-as-field";

export function ConversationTicketFields({
  form,
}: {
  form: UseFormReturn<ConversationValues>;
}) {
  const { t } = useTranslation();
  return (
    <div className="space-y-4 rounded-md border bg-muted/30 p-3">
      <ConversationRunAsField form={form} ticket />
      <FormField
        control={form.control}
        name="ticketAddressFrom"
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t("channels.ticketAddressFrom")}</FormLabel>
            <FormControl>
              <Input {...field} className="font-mono" placeholder="app:A0123TICKET" />
            </FormControl>
            <FormDescription>
              {t("channels.ticketAddressFromExplains")}
            </FormDescription>
            <FormMessage />
          </FormItem>
        )}
      />
      <FormField
        control={form.control}
        name="ticketPatterns"
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t("channels.ticketPatterns")}</FormLabel>
            <FormControl>
              <Textarea
                {...field}
                className="min-h-24 font-mono text-xs"
                placeholder={t("channels.ticketPatternsPlaceholder")}
              />
            </FormControl>
            <FormDescription>{t("channels.ticketPatternsExplains")}</FormDescription>
            <FormMessage />
          </FormItem>
        )}
      />
    </div>
  );
}
