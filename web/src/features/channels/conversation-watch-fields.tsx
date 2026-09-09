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
import { Textarea } from "@/components/ui/textarea";
import { usePeople } from "@/features/admin/people-api";
import type { ConversationValues } from "@/features/channels/conversation-form-model";
import { useMe } from "@/features/session/api";

type RunAsPerson = {
  id: string;
  display?: string | null;
  email?: string | null;
};

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
  const people = usePeople();
  const { data: me } = useMe();
  const peopleItems = (people.data?.items ?? []).filter((p) => !p.disabled);
  // Undefined is still loading; null is a session with no restrictions to
  // apply, which is how the other fields here read it too.
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
    <div className="rounded-md border bg-muted/30 p-3">
      <div className="grid gap-4">
        <FormField
          control={form.control}
          name="runAs"
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t("channels.watchRunAs")}</FormLabel>
              {runAsPeople.length > 0 ? (
                <Select
                  onValueChange={field.onChange}
                  value={field.value ?? ""}
                >
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
                  <Input
                    {...field}
                    placeholder={t("channels.watchRunAsPlaceholder")}
                  />
                </FormControl>
              )}
              <FormDescription>
                {canDelegate
                  ? t("channels.watchRunAsExplains")
                  : t("channels.watchRunAsSelfOnly")}
              </FormDescription>
              <FormMessage />
            </FormItem>
          )}
        />
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
