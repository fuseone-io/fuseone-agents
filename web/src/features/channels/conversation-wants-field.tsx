import type { UseFormReturn } from "react-hook-form";
import { useTranslation } from "react-i18next";
import { Checkbox } from "@/components/ui/checkbox";
import {
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from "@/components/ui/form";
import {
  EVENTS,
  type ConversationValues,
} from "@/features/channels/conversation-form-model";

/**
 * What this conversation is told about, and who else hears it.
 *
 * One component because they are one decision taken twice: sending the card
 * privately as well is offered where the events are chosen, and only where
 * parked runs are among them. A conversation never told a run stopped cannot
 * tell anybody privately either, and the switch would do nothing.
 */
export function ConversationWantsField({
  form,
}: {
  form: UseFormReturn<ConversationValues>;
}) {
  const { t } = useTranslation();

  return (
    <>
      <FormField
        control={form.control}
        name="wants"
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t("channels.wants")}</FormLabel>
            <div className="flex flex-col gap-2">
              {EVENTS.map((event) => (
                <label key={event} className="flex items-center gap-2 text-sm">
                  <Checkbox
                    checked={field.value.includes(event)}
                    onCheckedChange={(on) =>
                      field.onChange(
                        on
                          ? [...field.value, event]
                          : field.value.filter((e) => e !== event),
                      )
                    }
                  />
                  {t(`channels.event.${event}`)}
                </label>
              ))}
            </div>
            <FormDescription>{t("channels.wantsExplains")}</FormDescription>
            <FormMessage />
          </FormItem>
        )}
      />
      {form.watch("wants").includes("parked") && (
        <FormField
          control={form.control}
          name="directApprovals"
          render={({ field }) => (
            <FormItem className="rounded-md border bg-muted/30 p-3">
              <label className="flex items-start gap-2 text-sm">
                <Checkbox
                  checked={field.value}
                  onCheckedChange={(on) => field.onChange(Boolean(on))}
                />
                <span className="grid gap-1">
                  <span className="font-medium">
                    {t("channels.directApprovals")}
                  </span>
                  <span className="text-muted-foreground">
                    {t("channels.directApprovalsExplains")}
                  </span>
                </span>
              </label>
              <FormMessage />
            </FormItem>
          )}
        />
      )}
    </>
  );
}
