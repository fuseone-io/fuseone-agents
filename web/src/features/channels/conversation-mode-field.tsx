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
import type {
  ConversationMode,
  ConversationValues,
} from "@/features/channels/conversation-form-model";

/**
 * What may start a run here.
 *
 * A boundary rather than a label: a conversation that only watches configured
 * sources must not be startable by anybody who can type in it, and one set to
 * only report must not be startable at all.
 */
export function ConversationModeField({
  form,
  mode,
}: {
  form: UseFormReturn<ConversationValues>;
  mode: ConversationMode;
}) {
  const { t } = useTranslation();

  return (
    <FormField
      control={form.control}
      name="mode"
      render={({ field }) => (
        <FormItem>
          <FormLabel>{t("channels.startMode")}</FormLabel>
          <Select onValueChange={field.onChange} value={field.value ?? ""}>
            <FormControl>
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
            </FormControl>
            <SelectContent>
              <SelectItem value="mentions">
                {t("channels.modeMentions")}
              </SelectItem>
              <SelectItem value="watch">{t("channels.modeWatch")}</SelectItem>
              <SelectItem value="both">{t("channels.modeBoth")}</SelectItem>
              <SelectItem value="announce">
                {t("channels.modeAnnounce")}
              </SelectItem>
            </SelectContent>
          </Select>
          <FormDescription>
            {mode === "announce"
              ? t("channels.modeAnnounceExplains")
              : mode === "watch"
                ? t("channels.modeWatchExplains")
                : mode === "both"
                  ? t("channels.modeBothExplains")
                  : t("channels.modeMentionsExplains")}
          </FormDescription>
          <FormMessage />
        </FormItem>
      )}
    />
  );
}
