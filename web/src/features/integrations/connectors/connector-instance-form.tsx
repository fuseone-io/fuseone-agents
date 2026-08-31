import { zodResolver } from "@hookform/resolvers/zod";
import { useForm } from "react-hook-form";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from "@/components/ui/form";
import { Input } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import {
  PropertiesSheet,
  PropertiesSheetBody,
  PropertiesSheetFooter,
} from "@/components/shared/properties-sheet";
import { useActiveScope } from "@/features/scope/active-scope";
import type { ConnectorInstance } from "@/features/integrations/api";
import { ConnectorIdentityFields } from "@/features/integrations/connectors/connector-identity-fields";
import {
  connectorInstanceDefaults,
  connectorInstancePayload,
  connectorInstanceSchema,
  type ConnectorInstanceSaver,
  type ConnectorInstanceValues,
} from "@/features/integrations/connectors/connector-instance-model";
import { problemMessage } from "@/lib/api/problem-message";

type ConnectorTextField =
  | "name"
  | "address"
  | "mount"
  | "namespace"
  | "token";

export function ConnectorInstanceForm({
  instance,
  onClose,
  onSave,
}: {
  instance: ConnectorInstance | null;
  onClose: () => void;
  onSave: ConnectorInstanceSaver;
}) {
  const { t } = useTranslation();
  const company = useActiveScope((s) => s.company);
  const area = useActiveScope((s) => s.area);
  const form = useForm<ConnectorInstanceValues>({
    resolver: zodResolver(connectorInstanceSchema),
    mode: "onChange",
    defaultValues: connectorInstanceDefaults(instance, company, area),
  });

  async function submit(values: ConnectorInstanceValues) {
    const input = connectorInstancePayload(values, instance?.hasToken === true);
    if (!input) {
      form.setError("token", { message: "connectors.tokenRequired" });
      return;
    }
    try {
      await onSave(input);
      toast.success(t("connectors.instanceSaved"));
      onClose();
    } catch (error) {
      toast.error(problemMessage(error, t));
    }
  }

  return (
    <PropertiesSheet
      open
      onOpenChange={(open) => !open && onClose()}
      title={instance ? t("connectors.editInstance") : t("connectors.newVault")}
      description={t("connectors.instanceSheetHint")}
    >
      <Form {...form}>
        <form
          onSubmit={form.handleSubmit(submit)}
          className="flex min-h-0 flex-1 flex-col"
        >
          <PropertiesSheetBody className="space-y-4">
            <ConnectorIdentityFields editing={instance !== null} connector="vault" />
            <VaultFields form={form} hasToken={instance?.hasToken === true} />
          </PropertiesSheetBody>
          <PropertiesSheetFooter>
            <Button type="button" variant="outline" onClick={onClose}>
              {t("common.cancel")}
            </Button>
            <Button type="submit" disabled={form.formState.isSubmitting}>
              {t("common.save")}
            </Button>
          </PropertiesSheetFooter>
        </form>
      </Form>
    </PropertiesSheet>
  );
}

function VaultFields({
  form,
  hasToken,
}: {
  form: ReturnType<typeof useForm<ConnectorInstanceValues>>;
  hasToken: boolean;
}) {
  const { t } = useTranslation();
  return (
    <div className="grid gap-4">
      <TextField form={form} name="address" label={t("connectors.vaultAddress")} mono />
      <div className="grid gap-3 sm:grid-cols-2">
        <TextField form={form} name="mount" label={t("connectors.vaultMount")} mono />
        <TextField form={form} name="namespace" label={t("connectors.vaultNamespace")} mono />
      </div>
      <FormField
        control={form.control}
        name="allowedPathPrefixes"
        render={({ field }) => (
          <FormItem>
            <FormLabel>{t("connectors.allowedPathPrefixes")}</FormLabel>
            <FormControl>
              <Textarea {...field} className="min-h-24 font-mono" />
            </FormControl>
            <FormDescription>{t("connectors.allowedPathPrefixesHint")}</FormDescription>
            <FormMessage />
          </FormItem>
        )}
      />
      <TextField
        form={form}
        name="token"
        label={t("connectors.vaultToken")}
        type="password"
        description={hasToken ? t("connectors.tokenKept") : t("connectors.tokenNew")}
        mono
      />
      {hasToken && (
        <FormField
          control={form.control}
          name="clearToken"
          render={({ field }) => (
            <FormItem className="flex items-center justify-between rounded-lg border p-3">
              <FormLabel className="m-0">{t("connectors.clearToken")}</FormLabel>
              <FormControl>
                <Switch checked={field.value} onCheckedChange={field.onChange} />
              </FormControl>
            </FormItem>
          )}
        />
      )}
    </div>
  );
}

function TextField({
  form,
  name,
  label,
  description,
  type = "text",
  mono,
}: {
  form: ReturnType<typeof useForm<ConnectorInstanceValues>>;
  name: ConnectorTextField;
  label: string;
  description?: string;
  type?: string;
  mono?: boolean;
}) {
  return (
    <FormField
      control={form.control}
      name={name}
      render={({ field }) => (
        <FormItem>
          <FormLabel>{label}</FormLabel>
          <FormControl>
            <Input {...field} type={type} autoComplete="off" className={mono ? "font-mono" : undefined} />
          </FormControl>
          {description && <FormDescription>{description}</FormDescription>}
          <FormMessage />
        </FormItem>
      )}
    />
  );
}
