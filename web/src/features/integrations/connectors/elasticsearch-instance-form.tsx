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
import {
  PropertiesSheet,
  PropertiesSheetBody,
  PropertiesSheetFooter,
} from "@/components/shared/properties-sheet";
import { useActiveScope } from "@/features/scope/active-scope";
import { ConnectorIdentityFields } from "@/features/integrations/connectors/connector-identity-fields";
import type { ConnectorEditorProps } from "@/features/integrations/connectors/connector-editor-registry";
import {
  elasticsearchInstanceDefaults,
  elasticsearchInstancePayload,
  elasticsearchInstanceSchema,
  type ElasticsearchInstanceValues,
} from "@/features/integrations/connectors/elasticsearch-instance-model";
import { problemMessage } from "@/lib/api/problem-message";

export function ElasticsearchInstanceForm({
  instance,
  onClose,
  onSave,
}: ConnectorEditorProps) {
  const { t } = useTranslation();
  const company = useActiveScope((s) => s.company);
  const area = useActiveScope((s) => s.area);
  const form = useForm<ElasticsearchInstanceValues>({
    resolver: zodResolver(elasticsearchInstanceSchema),
    mode: "onChange",
    defaultValues: elasticsearchInstanceDefaults(instance, company, area),
  });

  async function submit(values: ElasticsearchInstanceValues) {
    const input = elasticsearchInstancePayload(
      values,
      instance?.hasToken === true,
    );
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

  const hasToken = instance?.hasToken === true;

  return (
    <PropertiesSheet
      open
      onOpenChange={(open) => !open && onClose()}
      title={
        instance
          ? t("connectors.editInstance")
          : t("connectors.newElasticsearch")
      }
      description={t("connectors.esSheetHint")}
    >
      <Form {...form}>
        <form
          onSubmit={form.handleSubmit(submit)}
          className="flex min-h-0 flex-1 flex-col"
        >
          <PropertiesSheetBody className="space-y-4">
            <ConnectorIdentityFields
              editing={instance !== null}
              connector="elasticsearch"
            />
            <Text
              form={form}
              name="baseUrl"
              label={t("connectors.esBaseUrl")}
            />
            <div className="grid gap-3 sm:grid-cols-2">
              <Text
                form={form}
                name="username"
                label={t("connectors.esUsername")}
              />
              <Text
                form={form}
                name="index"
                label={t("connectors.esIndex")}
                description={t("connectors.esIndexHint")}
              />
            </div>
            <div className="grid gap-3 sm:grid-cols-2">
              <Text
                form={form}
                name="pathField"
                label={t("connectors.esPathField")}
                description={t("connectors.esFieldHint")}
              />
              <Text
                form={form}
                name="ipField"
                label={t("connectors.esIpField")}
              />
              <Text
                form={form}
                name="statusField"
                label={t("connectors.esStatusField")}
              />
              <Text
                form={form}
                name="timestampField"
                label={t("connectors.esTimestampField")}
              />
            </div>
            <Text
              form={form}
              name="maxWindowDays"
              label={t("connectors.esMaxWindow")}
              description={t("connectors.esMaxWindowHint")}
              type="number"
            />
            <Text
              form={form}
              name="token"
              label={t("connectors.esPassword")}
              type="password"
              description={
                hasToken ? t("connectors.tokenKept") : t("connectors.tokenNew")
              }
            />
            {hasToken && (
              <FormField
                control={form.control}
                name="clearToken"
                render={({ field }) => (
                  <FormItem className="flex items-center justify-between rounded-lg border p-3">
                    <FormLabel className="m-0">
                      {t("connectors.clearToken")}
                    </FormLabel>
                    <FormControl>
                      <Switch
                        checked={field.value}
                        onCheckedChange={field.onChange}
                      />
                    </FormControl>
                  </FormItem>
                )}
              />
            )}
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

function Text({
  form,
  name,
  label,
  description,
  type = "text",
}: {
  form: ReturnType<typeof useForm<ElasticsearchInstanceValues>>;
  name:
    | "baseUrl"
    | "username"
    | "index"
    | "timestampField"
    | "ipField"
    | "pathField"
    | "statusField"
    | "maxWindowDays"
    | "token";
  label: string;
  description?: string;
  type?: string;
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
              {...field}
              type={type}
              autoComplete="off"
              className="font-mono"
            />
          </FormControl>
          {description && <FormDescription>{description}</FormDescription>}
          <FormMessage />
        </FormItem>
      )}
    />
  );
}
