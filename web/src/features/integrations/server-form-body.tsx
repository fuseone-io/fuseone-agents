import { Trans, useTranslation } from "react-i18next";
import type { UseFormReturn } from "react-hook-form";
import { Input } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from "@/components/ui/form";
import { ServerFields } from "@/features/integrations/server-fields";
import type { ServerFormValues } from "@/features/integrations/server-schema";

/** Everything a tool server is, in the order somebody fills it: what it is
 *  called, how it is reached, and then whichever half that answer needs. */
export function ServerFormBody({
  form,
  editing,
  hasSecret,
}: {
  form: UseFormReturn<ServerFormValues>;
  editing: boolean;
  hasSecret: boolean;
}) {
  const { t } = useTranslation();

  return (
    <>
      <FormField
        control={form.control}
        name="name"
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t("admin.name")}</FormLabel>
            <FormControl>
              <Input {...field} disabled={editing} className="font-mono" />
            </FormControl>
            <FormDescription>
              <Trans
                i18nKey="integrations.prefixExplains"
                components={{ code: <code className="font-mono" /> }}
              />
            </FormDescription>
            <FormMessage />
          </FormItem>
        )}
      />

      <FormField
        control={form.control}
        name="transport"
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t("integrations.transport")}</FormLabel>
            <Select value={field.value} onValueChange={field.onChange}>
              <FormControl>
                <SelectTrigger className="w-full">
                  <SelectValue />
                </SelectTrigger>
              </FormControl>
              <SelectContent>
                <SelectItem value="stdio">
                  {t("integrations.transportStdio")}
                </SelectItem>
                <SelectItem value="http">
                  {t("integrations.transportHTTP")}
                </SelectItem>
              </SelectContent>
            </Select>
            <FormDescription>{t("integrations.transportHint")}</FormDescription>
            <FormMessage />
          </FormItem>
        )}
      />

      <ServerFields form={form} hasSecret={hasSecret} />

      <FormField
        control={form.control}
        name="enabled"
        render={({ field }) => (
          <FormItem className="flex items-center justify-between rounded-lg border p-3">
            <FormLabel className="m-0">{t("integrations.enabled")}</FormLabel>
            <FormControl>
              <Switch checked={field.value} onCheckedChange={field.onChange} />
            </FormControl>
          </FormItem>
        )}
      />
    </>
  );
}
