import { useEffect } from "react";
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
import { usePeople } from "@/features/admin/people-api";
import type { ConversationValues } from "@/features/channels/conversation-form-model";
import { useMe } from "@/features/session/api";

type RunAsPerson = {
  id: string;
  display?: string | null;
  email?: string | null;
};

// The principal pinned to an automated inbound path. Slack identity selects
// neither authority nor credentials; delegation remains identity governance.
export function ConversationRunAsField({
  form,
  ticket = false,
}: {
  form: UseFormReturn<ConversationValues>;
  ticket?: boolean;
}) {
  const { t } = useTranslation();
  const people = usePeople();
  const { data: me } = useMe();
  const peopleItems = (people.data?.items ?? []).filter((p) => !p.disabled);
  const canDelegate =
    me === null || Boolean(me?.can.includes("identity:write"));
  const runAsPeople: RunAsPerson[] = canDelegate
    ? peopleItems
    : me
      ? peopleItems.some((person) => person.id === me.id)
        ? peopleItems.filter((person) => person.id === me.id)
        : [{ id: me.id, display: me.display }]
      : [];

  useEffect(() => {
    if (me === null || me === undefined || canDelegate) return;
    if (form.getValues("runAs") === me.id) return;
    form.setValue("runAs", me.id, { shouldValidate: true });
  }, [canDelegate, form, me]);

  return (
    <FormField
      control={form.control}
      name="runAs"
      render={({ field }) => (
        <FormItem>
          <FormLabel>{t("channels.watchRunAs")}</FormLabel>
          {runAsPeople.length > 0 ? (
            <Select onValueChange={field.onChange} value={field.value ?? ""}>
              <FormControl>
                <SelectTrigger>
                  <SelectValue placeholder={t("channels.pickWatchRunAs")} />
                </SelectTrigger>
              </FormControl>
              <SelectContent>
                {runAsPeople.map((person) => (
                  <SelectItem key={person.id} value={person.id}>
                    {person.display || person.email || person.id}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          ) : me === undefined ? (
            <Select disabled value="">
              <FormControl>
                <SelectTrigger>
                  <SelectValue placeholder={t("common.loading")} />
                </SelectTrigger>
              </FormControl>
            </Select>
          ) : (
            <FormControl>
              <Input {...field} placeholder={t("channels.watchRunAsPlaceholder")} />
            </FormControl>
          )}
          <FormDescription>
            {canDelegate
              ? t(
                  ticket
                    ? "channels.ticketRunAsExplains"
                    : "channels.watchRunAsExplains",
                )
              : t(
                  ticket
                    ? "channels.ticketRunAsSelfOnly"
                    : "channels.watchRunAsSelfOnly",
                )}
          </FormDescription>
          <FormMessage />
        </FormItem>
      )}
    />
  );
}
