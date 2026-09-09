import type { UseFormReturn } from "react-hook-form";
import { useTranslation } from "react-i18next";
import { Checkbox } from "@/components/ui/checkbox";
import { FormField, FormItem, FormMessage } from "@/components/ui/form";
import type { ConversationValues } from "@/features/channels/conversation-form-model";

/**
 * How much of the surrounding thread travels with a mention.
 *
 * Not authority — the conversation already speaks for a scope — but a decision
 * about how much untrusted text goes into the run input, which is why it is
 * asked rather than assumed.
 */
export function ConversationThreadContextField({
  form,
}: {
  form: UseFormReturn<ConversationValues>;
}) {
  const { t } = useTranslation();

  return (
    <FormField
      control={form.control}
      name="threadContext"
      render={({ field }) => (
        <FormItem className="rounded-md border bg-muted/30 p-3">
          <label className="flex items-start gap-2 text-sm">
            <Checkbox
              checked={field.value}
              onCheckedChange={(on) => field.onChange(Boolean(on))}
            />
            <span className="grid gap-1">
              <span className="font-medium">{t("channels.threadContext")}</span>
              <span className="text-muted-foreground">
                {t("channels.threadContextExplains")}
              </span>
            </span>
          </label>
          <FormMessage />
        </FormItem>
      )}
    />
  );
}
