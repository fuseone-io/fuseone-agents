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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { useScopes } from "@/features/scope/api";
import { useMe } from "@/features/session/api";
import type { ConversationValues } from "@/features/channels/conversation-form-model";

// The scope above every company, written the way the API writes it. A
// conversation there hears about a run in any company, which is what makes it
// the answer for an area nobody has pointed at a channel of its own.
export const INSTALLATION_SCOPE = "*/";

/**
 * Which runs report here.
 *
 * The governing field, and a choice rather than typed text: a conversation
 * receives the runs of the scope it is configured in and no others, so a free
 * text box would be a way to name an area somebody cannot otherwise see and
 * have its runs delivered to them.
 *
 * The installation is offered only to whoever governs it. Configuring where one
 * area reports is an ordinary scoped act; deciding that one room hears every
 * company is the authority above them all, and the server refuses it from
 * anybody else — so offering it here would be a control that answers 403.
 */
export function ConversationScopeField({
  form,
}: {
  form: UseFormReturn<ConversationValues>;
}) {
  const { t } = useTranslation();
  const { data: scopes } = useScopes();
  const { data: me } = useMe();
  // Undefined is still loading; null is a session with no restrictions to
  // apply, which is how the other fields here read it too.
  const governs = me === null || Boolean(me?.can.includes("company:write"));

  return (
    <FormField
      control={form.control}
      name="scope"
      render={({ field }) => (
        <FormItem>
          <FormLabel>{t("scope.label")}</FormLabel>
          <Select
            onValueChange={(picked) => {
              field.onChange(picked);
              // An agent is startable only in the scope it is published in, so
              // one chosen for the old scope is a configuration the server
              // refuses. Cleared in the same act rather than left on screen
              // until save fails.
              form.setValue("agent", "", { shouldValidate: true });
            }}
            value={field.value ?? ""}
          >
            <FormControl>
              <SelectTrigger>
                <SelectValue placeholder={t("scope.label")} />
              </SelectTrigger>
            </FormControl>
            <SelectContent>
              {governs && (
                <SelectItem value={INSTALLATION_SCOPE}>
                  {t("channels.installationScope")}
                </SelectItem>
              )}
              {(scopes?.items ?? []).map((s) => (
                <SelectItem
                  key={`${s.company}/${s.area}`}
                  value={`${s.company}/${s.area}`}
                >
                  {s.label || `${s.company}/${s.area}`}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <FormDescription>
            {field.value === INSTALLATION_SCOPE
              ? t("channels.installationScopeExplains")
              : t("channels.scopeGoverns")}
          </FormDescription>
          <FormMessage />
        </FormItem>
      )}
    />
  );
}
