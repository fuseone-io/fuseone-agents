import { useTranslation } from "react-i18next";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";
import {
  PropertiesSheet,
  PropertiesSheetBody,
  PropertiesSheetFooter,
} from "@/components/shared/properties-sheet";
import {
  Form,
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
import {
  useIntegrations,
  usePutProvider,
  type ModelProvider,
} from "@/features/integrations/api";
import { problemMessage } from "@/lib/api/problem-message";

const schema = z.object({
  name: z.string().min(1, "integrations.nameProvider"),
  kind: z.enum(["anthropic", "openai_compatible"]),
  // Optional, and the reason is not laziness. Anthropic's preset carries no
  // address because its client already knows one, and a self-hosted model's
  // is known only to the installation. Requiring it made the reference
  // provider impossible to configure at all.
  baseUrl: z.string(),
  apiKey: z.string(),
  enabled: z.boolean(),
});

export function ProviderForm({
  provider,
  onClose,
}: {
  provider: ModelProvider | null;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const put = usePutProvider();
  const presets = useIntegrations().data?.presets ?? [];
  const form = useForm<z.infer<typeof schema>>({
    resolver: zodResolver(schema),
    defaultValues: {
      name: provider?.name ?? "",
      kind:
        (provider?.kind as "anthropic" | "openai_compatible") ??
        "openai_compatible",
      baseUrl: provider?.baseUrl ?? "",
      apiKey: "",
      enabled: provider?.enabled ?? true,
    },
  });

  // Choosing a provider the platform already knows fills in its protocol and
  // its endpoint. An operator should have to type an address only for a proxy
  // or a self-hosted model — the two cases where the installation knows
  // something the platform cannot.
  const applyPreset = (name: string) => {
    form.setValue("name", name);
    const preset = presets.find((p) => p.name === name);
    if (!preset) return;
    form.setValue("kind", preset.kind as "anthropic" | "openai_compatible");
    form.setValue("baseUrl", preset.baseUrl ?? "");
  };

  async function submit(values: z.infer<typeof schema>) {
    // An address is needed where the client does not already know one, which
    // is every OpenAI-compatible provider including the self-hosted ones.
    if (values.kind === "openai_compatible" && !values.baseUrl.trim()) {
      form.setError("baseUrl", { message: "integrations.addressNeeded" });
      return;
    }
    try {
      await put.mutateAsync({ ...values, apiKey: values.apiKey || undefined });
      toast.success(t("integrations.saved", { name: values.name }));
      onClose();
    } catch (error) {
      toast.error(
        problemMessage(error, t),
      );
    }
  }

  return (
    <PropertiesSheet
      open
      onOpenChange={(open) => !open && onClose()}
      title={
        provider
          ? t("integrations.editProvider")
          : t("integrations.newProvider")
      }
      description={t("integrations.credentialSealed")}
    >
      <Form {...form}>
        <form
          onSubmit={form.handleSubmit(submit)}
          className="flex min-h-0 flex-1 flex-col"
        >
          <PropertiesSheetBody className="space-y-4">
            <FormField
              control={form.control}
              name="name"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t("admin.name")}</FormLabel>
                  <FormControl>
                    <div className="flex gap-2">
                      <Input
                        {...field}
                        disabled={!!provider}
                        // Off, or the browser offers whatever was typed into
                        // any similarly named field before — and a list mixing
                        // "deepseek" with somebody's old form entry reads as
                        // the platform suggesting providers that do not exist.
                        autoComplete="off"
                        className="font-mono"
                        placeholder="openai"
                      />
                      {!provider && (
                        <Select onValueChange={applyPreset}>
                          <SelectTrigger className="w-[150px] shrink-0">
                            <SelectValue
                              placeholder={t("integrations.known")}
                            />
                          </SelectTrigger>
                          <SelectContent>
                            {presets.map((preset) => (
                              <SelectItem
                                key={preset.name}
                                value={preset.name}
                                className="font-mono"
                              >
                                {preset.name}
                              </SelectItem>
                            ))}
                          </SelectContent>
                        </Select>
                      )}
                    </div>
                  </FormControl>
                  <FormDescription>
                    {t("integrations.referencedBySpecs")}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name="kind"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t("integrations.protocol")}</FormLabel>
                  <Select value={field.value} onValueChange={field.onChange}>
                    <FormControl>
                      <SelectTrigger>
                        <SelectValue />
                      </SelectTrigger>
                    </FormControl>
                    <SelectContent>
                      {/* eslint-disable-next-line i18next/no-literal-string */}
                      <SelectItem value="anthropic">Anthropic</SelectItem>
                      <SelectItem value="openai_compatible">
                        {t("integrations.openAiCompatible")}
                      </SelectItem>
                    </SelectContent>
                  </Select>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name="baseUrl"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t("integrations.address")}</FormLabel>
                  <FormControl>
                    <Input
                      {...field}
                      className="font-mono"
                      placeholder={
                        form.watch("kind") === "anthropic"
                          ? t("integrations.clientKnowsIt")
                          : "https://api.openai.com/v1"
                      }
                    />
                  </FormControl>
                  {/* Said where it applies rather than always: the address is
                      for a proxy or a self-hosted model, and a field that
                      looks mandatory is one somebody fills with a guess. */}
                  <FormDescription>
                    {t(
                      form.watch("kind") === "anthropic"
                        ? "integrations.addressOnlyForProxy"
                        : "integrations.addressFilledByPreset",
                    )}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name="apiKey"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t("integrations.credential")}</FormLabel>
                  <FormControl>
                    <Input
                      {...field}
                      type="password"
                      autoComplete="off"
                      className="font-mono"
                    />
                  </FormControl>
                  <FormDescription>
                    {provider?.hasKey
                      ? t("integrations.credentialKept")
                      : t("integrations.credentialVault")}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name="enabled"
              render={({ field }) => (
                <FormItem className="flex items-center justify-between rounded-lg border p-3">
                  <FormLabel className="m-0">
                    {t("integrations.enabled")}
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
