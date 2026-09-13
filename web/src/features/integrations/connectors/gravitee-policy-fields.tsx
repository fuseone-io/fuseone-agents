import type { useForm } from "react-hook-form";
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
import type { GraviteeInstanceValues } from "@/features/integrations/connectors/gravitee-instance-model";

export function GraviteePolicyFields({
  form,
}: {
  form: ReturnType<typeof useForm<GraviteeInstanceValues>>;
}) {
  const { t } = useTranslation();
  return (
    <section className="grid gap-4 border-t pt-5">
      <div>
        <h3 className="text-sm font-medium">{t("connectors.graviteeExpiryPolicy")}</h3>
        <p className="text-xs text-muted-foreground">{t("connectors.graviteeExpiryPolicyHint")}</p>
      </div>
      <div className="grid gap-3 sm:grid-cols-2">
        <TTLField form={form} name="minTTLSeconds" label={t("connectors.graviteeMinTTL")} />
        <TTLField form={form} name="maxTTLSeconds" label={t("connectors.graviteeMaxTTL")} />
      </div>
      <FormField
        control={form.control}
        name="allowNoExpiry"
        render={({ field }) => (
          <FormItem className="flex items-start justify-between gap-4 rounded-lg border p-3">
            <div>
              <FormLabel className="m-0">{t("connectors.graviteeAllowNoExpiry")}</FormLabel>
              <FormDescription>{t("connectors.graviteeAllowNoExpiryHint")}</FormDescription>
            </div>
            <FormControl>
              <Switch checked={field.value} onCheckedChange={field.onChange} />
            </FormControl>
          </FormItem>
        )}
      />
    </section>
  );
}

function TTLField({
  form,
  name,
  label,
}: {
  form: ReturnType<typeof useForm<GraviteeInstanceValues>>;
  name: "minTTLSeconds" | "maxTTLSeconds";
  label: string;
}) {
  return (
    <FormField
      control={form.control}
      name={name}
      render={({ field }) => (
        <FormItem>
          <FormLabel>{label}</FormLabel>
          <FormControl>
            <Input
              type="number"
              min={1}
              max={31_536_000}
              value={field.value}
              onBlur={field.onBlur}
              onChange={(event) => field.onChange(event.target.valueAsNumber)}
            />
          </FormControl>
          <FormMessage />
        </FormItem>
      )}
    />
  );
}
