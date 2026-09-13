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
import type { GraviteeInstanceValues } from "@/features/integrations/connectors/gravitee-instance-model";

type TargetName = "address" | "organization" | "environment" | "apiReference";

export function GraviteeTargetFields({
  form,
}: {
  form: ReturnType<typeof useForm<GraviteeInstanceValues>>;
}) {
  const { t } = useTranslation();
  return (
    <section className="grid gap-4 border-t pt-5">
      <div>
        <h3 className="text-sm font-medium">{t("connectors.graviteeTarget")}</h3>
        <p className="text-xs text-muted-foreground">{t("connectors.graviteeTargetHint")}</p>
      </div>
      <TargetField
        form={form}
        name="address"
        label={t("connectors.graviteeAddress")}
        description={t("connectors.graviteeAddressHint")}
      />
      <div className="grid gap-3 sm:grid-cols-2">
        <TargetField form={form} name="organization" label={t("connectors.graviteeOrganization")} />
        <TargetField form={form} name="environment" label={t("connectors.graviteeEnvironment")} />
      </div>
      <TargetField
        form={form}
        name="apiReference"
        label={t("connectors.graviteeAPIReference")}
        description={t("connectors.graviteeAPIReferenceHint")}
      />
    </section>
  );
}

function TargetField({
  form,
  name,
  label,
  description,
}: {
  form: ReturnType<typeof useForm<GraviteeInstanceValues>>;
  name: TargetName;
  label: string;
  description?: string;
}) {
  return (
    <FormField
      control={form.control}
      name={name}
      render={({ field }) => (
        <FormItem>
          <FormLabel>{label}</FormLabel>
          <FormControl><Input {...field} autoComplete="off" className="font-mono" /></FormControl>
          {description && <FormDescription>{description}</FormDescription>}
          <FormMessage />
        </FormItem>
      )}
    />
  );
}
