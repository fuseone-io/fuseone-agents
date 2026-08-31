import { zodResolver } from "@hookform/resolvers/zod";
import { AlertTriangle } from "lucide-react";
import { useMemo } from "react";
import { useForm } from "react-hook-form";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  PropertiesSheet,
  PropertiesSheetBody,
  PropertiesSheetFooter,
} from "@/components/shared/properties-sheet";
import type {
  ConnectorInstance,
  ConnectorInstanceDetail,
} from "@/features/integrations/api";
import { ConnectorIdentityFields } from "@/features/integrations/connectors/connector-identity-fields";
import type { ConnectorInstanceSaver } from "@/features/integrations/connectors/connector-instance-model";
import {
  sqlInstanceDefaults,
  sqlInstancePayload,
  sqlInstanceSchema,
  vaultChoices,
  type SQLInstanceValues,
} from "@/features/integrations/connectors/sql-instance-model";
import { SQLTemplateEditor } from "@/features/integrations/connectors/sql-template-editor";
import { useActiveScope } from "@/features/scope/active-scope";
import { problemMessage } from "@/lib/api/problem-message";

export function SQLInstanceForm({
  instance,
  instances,
  onClose,
  onSave,
}: {
  instance: ConnectorInstanceDetail | null;
  instances: ConnectorInstance[];
  onClose: () => void;
  onSave: ConnectorInstanceSaver;
}) {
  const { t } = useTranslation();
  const company = useActiveScope((state) => state.company);
  const area = useActiveScope((state) => state.area);
  const form = useForm<SQLInstanceValues>({
    resolver: zodResolver(sqlInstanceSchema, undefined, { mode: "sync" }),
    mode: "onChange",
    defaultValues: sqlInstanceDefaults(instance, company, area),
  });
  const scopeKind = form.watch("scopeKind");
  const targetCompany = form.watch("company");
  const targetArea = form.watch("area");
  const choices = useMemo(
    () => vaultChoices(instances, {
      scopeKind,
      company: targetCompany,
      area: targetArea,
    }),
    [instances, scopeKind, targetCompany, targetArea],
  );

  async function submit(values: SQLInstanceValues) {
    const choice = choices.find((candidate) => candidate.name === values.vaultInstance);
    if (values.enabled && (!choice || choice.ambiguous)) {
      form.setError("vaultInstance", {
        message: choice?.ambiguous
          ? "connectors.sqlVaultAmbiguous"
          : "connectors.sqlVaultUnavailable",
      });
      return;
    }
    try {
      await onSave(sqlInstancePayload(values));
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
      title={instance ? t("connectors.editSQLInstance") : t("connectors.newSQL")}
      description={t("connectors.sqlInstanceSheetHint")}
      className="lg:max-w-[880px]"
    >
      <Form {...form}>
        <form
          onSubmit={form.handleSubmit(submit)}
          className="flex min-h-0 flex-1 flex-col"
        >
          <PropertiesSheetBody className="space-y-6">
            <ConnectorIdentityFields editing={instance !== null} connector="sql" />
            <SQLTargetFields form={form} />
            <VaultBindingFields form={form} choices={choices} />
            <SQLTemplateEditor form={form} />
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

function SQLTargetFields({ form }: { form: ReturnType<typeof useForm<SQLInstanceValues>> }) {
  const { t } = useTranslation();
  return (
    <section className="grid gap-4 border-t pt-5">
      <div>
        <h3 className="text-sm font-medium">{t("connectors.sqlTarget")}</h3>
        <p className="text-xs text-muted-foreground">{t("connectors.sqlTargetHint")}</p>
      </div>
      <div className="grid gap-3 sm:grid-cols-[minmax(0,1fr)_8rem]">
        <SQLTextField form={form} name="host" label={t("connectors.sqlHost")} />
        <FormField
          control={form.control}
          name="port"
          render={({ field }) => (
            <FormItem>
              <FormLabel>{t("connectors.sqlPort")}</FormLabel>
              <FormControl>
                <Input
                  type="number"
                  min={1}
                  max={65_535}
                  value={field.value}
                  onBlur={field.onBlur}
                  onChange={(event) => field.onChange(event.target.valueAsNumber)}
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
      </div>
      <SQLTextField form={form} name="database" label={t("connectors.sqlDatabase")} />
    </section>
  );
}

function VaultBindingFields({
  form,
  choices,
}: {
  form: ReturnType<typeof useForm<SQLInstanceValues>>;
  choices: ReturnType<typeof vaultChoices>;
}) {
  const { t } = useTranslation();
  const selected = form.watch("vaultInstance");
  const selectedListed = choices.some((choice) => choice.name === selected);
  return (
    <section className="grid gap-4 border-t pt-5">
      <div>
        <h3 className="text-sm font-medium">{t("connectors.sqlCredentialSource")}</h3>
        <p className="text-xs text-muted-foreground">{t("connectors.sqlCredentialSourceHint")}</p>
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
                  <SelectItem value={selected} disabled>{selected} · {t("connectors.unavailable")}</SelectItem>
                )}
                {choices.map((choice) => (
                  <SelectItem key={choice.name} value={choice.name} disabled={choice.ambiguous}>
                    {choice.label}{choice.ambiguous ? ` · ${t("connectors.ambiguous")}` : ""}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <FormDescription>{t("connectors.vaultInstanceHint")}</FormDescription>
            <FormMessage />
          </FormItem>
        )}
      />
      <div className="grid gap-3 sm:grid-cols-2">
        <SQLTextField form={form} name="credentialMount" label={t("connectors.sqlCredentialMount")} />
        <SQLTextField form={form} name="credentialRole" label={t("connectors.sqlCredentialRole")} />
      </div>
    </section>
  );
}

type SQLTextName = "host" | "database" | "credentialMount" | "credentialRole";

function SQLTextField({
  form,
  name,
  label,
}: {
  form: ReturnType<typeof useForm<SQLInstanceValues>>;
  name: SQLTextName;
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
            <Input {...field} autoComplete="off" className="font-mono" />
          </FormControl>
          <FormMessage />
        </FormItem>
      )}
    />
  );
}
