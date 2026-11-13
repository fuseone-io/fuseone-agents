import { useFormContext } from "react-hook-form";
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
import { Switch } from "@/components/ui/switch";
import { ScopeFields } from "@/features/integrations/scope/scope-fields";
import type { ScopeValues } from "@/features/integrations/scope/scope-choice";

export type ConnectorIdentityValues = ScopeValues & {
  name: string;
  enabled: boolean;
};

export function ConnectorIdentityFields({
  editing,
  connector,
}: {
  editing: boolean;
  connector: string;
}) {
  const { t } = useTranslation();
  const form = useFormContext<ConnectorIdentityValues>();
  return (
    <div className="grid gap-4">
      <FormField
        control={form.control}
        name="name"
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t("connectors.instanceName")}</FormLabel>
            <FormControl>
              <Input {...field} disabled={editing} className="font-mono" />
            </FormControl>
            <FormDescription>
              {t("connectors.instanceNameHint", { connector })}
            </FormDescription>
            <FormMessage />
          </FormItem>
        )}
      />
      <ScopeFields
        editing={editing}
        labels={{ kind: "connectors.scopeKind", held: "connectors.scopeHeld" }}
      />
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
    </div>
  );
}
