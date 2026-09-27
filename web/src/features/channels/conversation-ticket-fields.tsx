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
import { Textarea } from "@/components/ui/textarea";
import {
  marksThreads,
  type ConversationValues,
} from "@/features/channels/conversation-form-model";
import { ConversationReviewField } from "@/features/channels/conversation-review-field";
import { ConversationRunAsField } from "@/features/channels/conversation-run-as-field";

export function ConversationTicketFields({
  form,
  channel,
}: {
  form: UseFormReturn<ConversationValues>;
  channel: string;
}) {
  const { t } = useTranslation();
  const policy = form.watch("ticketOpenFrom");
  return (
    <div className="space-y-4 rounded-md border bg-muted/30 p-3">
      <ConversationRunAsField form={form} ticket />
      <ConversationReviewField form={form} channel={channel} />
      <FormField
        control={form.control}
        name="ticketOpenFrom"
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t("channels.ticketOpenFrom")}</FormLabel>
            <Select onValueChange={field.onChange} value={field.value}>
              <FormControl>
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
              </FormControl>
              <SelectContent>
                <SelectItem value="linked_users">
                  {t("channels.ticketOpenLinkedUsers")}
                </SelectItem>
                <SelectItem value="marked_threads">
                  {t("channels.ticketOpenMarkedThreads")}
                </SelectItem>
              </SelectContent>
            </Select>
            <FormDescription>
              {marksThreads(field.value)
                ? t("channels.ticketOpenMarkedThreadsExplains")
                : t("channels.ticketOpenLinkedUsersExplains")}
            </FormDescription>
            <FormMessage />
          </FormItem>
        )}
      />
      {marksThreads(policy) ? <ConversationTicketRootField form={form} /> : null}
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
        name="ticketClosesOn"
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t("channels.ticketClosesOn")}</FormLabel>
            <FormControl>
              <Input {...field} className="font-mono" placeholder="white_check_mark" />
            </FormControl>
            <FormDescription>{t("channels.ticketClosesOnExplains")}</FormDescription>
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
            <FormDescription>
              {marksThreads(policy)
                ? t("channels.ticketMarkPatternsExplains")
                : t("channels.ticketPatternsExplains")}
            </FormDescription>
            <FormMessage />
          </FormItem>
        )}
      />
    </div>
  );
}

function ConversationTicketRootField({
  form,
}: {
  form: UseFormReturn<ConversationValues>;
}) {
  const { t } = useTranslation();
  return (
    <FormField
      control={form.control}
      name="ticketRootFrom"
      render={({ field }) => (
        <FormItem>
          <FormLabel>{t("channels.ticketRootFrom")}</FormLabel>
          <FormControl>
            <Input {...field} className="font-mono" placeholder="bot:B0123FORMS" />
          </FormControl>
          <FormDescription>{t("channels.ticketRootFromExplains")}</FormDescription>
          <FormMessage />
        </FormItem>
      )}
    />
  );
}
