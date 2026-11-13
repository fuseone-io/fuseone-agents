import { useEffect } from "react";
import { useTranslation } from "react-i18next";
import { useWatch, type UseFormReturn } from "react-hook-form";
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
import type { ServerFormValues } from "@/features/integrations/server-schema";

/**
 * How the server is reached. Outside the installation only a remote address
 * is offered (NT-019): a local process runs on the platform's worker, which
 * no company or area commands, and the server refuses one stored there.
 */
export function ServerTransportField({
  form,
}: {
  form: UseFormReturn<ServerFormValues>;
}) {
  const { t } = useTranslation();
  const scopeKind = useWatch({ control: form.control, name: "scopeKind" });
  const transport = useWatch({ control: form.control, name: "transport" });
  const remoteOnly = scopeKind !== "installation";
  useEffect(() => {
    if (remoteOnly && transport === "stdio") {
      form.setValue("transport", "http", { shouldDirty: true });
    }
  }, [remoteOnly, transport, form]);

  return (
    <FormField
      control={form.control}
      name="transport"
      render={({ field }) => (
        <FormItem>
          <FormLabel>{t("integrations.transport")}</FormLabel>
          {/* An empty value is the select clearing itself when the value is
              set from code before its items were ever opened, never a
              choice: there is no server without a transport. */}
          <Select
            value={field.value}
            onValueChange={(value) => value && field.onChange(value)}
          >
            <FormControl>
              <SelectTrigger className="w-full">
                <SelectValue />
              </SelectTrigger>
            </FormControl>
            <SelectContent>
              {/* Disabled rather than dropped: a value with no item left
                  is one the select clears on its own. */}
              <SelectItem value="stdio" disabled={remoteOnly}>
                {t("integrations.transportStdio")}
              </SelectItem>
              <SelectItem value="http">
                {t("integrations.transportHTTP")}
              </SelectItem>
            </SelectContent>
          </Select>
          <FormDescription>
            {remoteOnly
              ? t("integrations.remoteOutsideInstallation")
              : t("integrations.transportHint")}
          </FormDescription>
          <FormMessage />
        </FormItem>
      )}
    />
  );
}
