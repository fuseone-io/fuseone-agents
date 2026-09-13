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
import { Textarea } from "@/components/ui/textarea";
import type { ConversationValues } from "@/features/channels/conversation-form-model";
import { ConversationRunAsField } from "@/features/channels/conversation-run-as-field";

/**
 * Who a watched message runs as, and which sources may start one.
 *
 * The principal is configured beforehand and is never read out of the message:
 * a Slack bot id does not become a FuseOne user. Delegating it to somebody else
 * is identity administration, so whoever cannot do that is offered themselves
 * and nobody else — and is set to themselves, rather than left with an empty
 * field the server would refuse.
 */
export function ConversationWatchFields({
  form,
}: {
  form: UseFormReturn<ConversationValues>;
}) {
  const { t } = useTranslation();

  return (
    <div className="rounded-md border bg-muted/30 p-3">
      <div className="grid gap-4">
        <ConversationRunAsField form={form} />
      </div>
      <FormField
        control={form.control}
        name="sources"
        render={({ field }) => (
          <FormItem className="mt-4">
            <FormLabel>{t("channels.watchSources")}</FormLabel>
            <FormControl>
              <Textarea
                {...field}
                className="min-h-20 font-mono text-xs"
                placeholder={t("channels.watchSourcesPlaceholder")}
              />
            </FormControl>
            <FormDescription>
              {t("channels.watchSourcesExplains")}
            </FormDescription>
            <FormMessage />
          </FormItem>
        )}
      />
    </div>
  );
}
