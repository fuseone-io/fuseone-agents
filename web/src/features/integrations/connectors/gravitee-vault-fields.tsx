import { AlertTriangle } from "lucide-react";
import { useWatch, type useForm } from "react-hook-form";
import { useTranslation } from "react-i18next";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
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
import type { GraviteeInstanceValues } from "@/features/integrations/connectors/gravitee-instance-model";
import type { VaultChoice } from "@/features/integrations/connectors/vault-instance-options";

export function GraviteeVaultFields({
  form,
  choices,
}: {
  form: ReturnType<typeof useForm<GraviteeInstanceValues>>;
  choices: VaultChoice[];
}) {
  const { t } = useTranslation();
  const selected = useWatch({ control: form.control, name: "vaultInstance" });
  const selectedListed = choices.some((choice) => choice.name === selected);
  return (
    <section className="grid gap-4 border-t pt-5">
      <div>
        <h3 className="text-sm font-medium">{t("connectors.graviteeCredentialSource")}</h3>
        <p className="text-xs text-muted-foreground">{t("connectors.graviteeCredentialSourceHint")}</p>
      </div>
      {choices.length === 0 && (
        <Alert>
          <AlertTriangle aria-hidden />
          <AlertTitle>{t("connectors.noUsableVault")}</AlertTitle>
          <AlertDescription>{t("connectors.noUsableVaultHint")}</AlertDescription>
        </Alert>
      )}
      <FormField
        control={form.control}
        name="vaultInstance"
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t("connectors.vaultInstance")}</FormLabel>
            <Select value={field.value} onValueChange={field.onChange}>
              <FormControl>
                <SelectTrigger><SelectValue placeholder={t("connectors.chooseVault")} /></SelectTrigger>
              </FormControl>
              <SelectContent>
                {selected && !selectedListed && (
                  <SelectItem value={selected} disabled>
                    {selected} · {t("connectors.unavailable")}
                  </SelectItem>
                )}
                {choices.map((choice) => (
                  <SelectItem key={choice.name} value={choice.name} disabled={choice.ambiguous}>
                    {choice.label}{choice.ambiguous ? ` · ${t("connectors.ambiguous")}` : ""}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <FormDescription>{t("connectors.graviteeVaultInstanceHint")}</FormDescription>
            <FormMessage />
          </FormItem>
        )}
      />
      <div className="grid gap-3 sm:grid-cols-2">
        <VaultTextField form={form} name="credentialPath" label={t("connectors.graviteeCredentialPath")} />
        <VaultTextField form={form} name="credentialField" label={t("connectors.graviteeCredentialField")} />
      </div>
    </section>
  );
}

function VaultTextField({
  form,
  name,
  label,
}: {
  form: ReturnType<typeof useForm<GraviteeInstanceValues>>;
  name: "credentialPath" | "credentialField";
  label: string;
}) {
  return (
    <FormField
      control={form.control}
      name={name}
      render={({ field }) => (
        <FormItem>
          <FormLabel>{label}</FormLabel>
          <FormControl><Input {...field} autoComplete="off" className="font-mono" /></FormControl>
          <FormMessage />
        </FormItem>
      )}
    />
  );
}
