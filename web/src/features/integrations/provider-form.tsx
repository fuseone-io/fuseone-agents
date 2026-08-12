import { useTranslation } from "react-i18next";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
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
  usePutProvider,
  type ModelProvider,
} from "@/features/integrations/api";

const schema = z.object({
  name: z.string().min(1, "Dê um nome ao provedor."),
  kind: z.enum(["anthropic", "openai_compatible"]),
  baseUrl: z.string().url("Precisa ser uma URL completa."),
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

  async function submit(values: z.infer<typeof schema>) {
    try {
      await put.mutateAsync({ ...values, apiKey: values.apiKey || undefined });
      toast.success(`${values.name} configurado`);
      onClose();
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : t("common.saveFailed"),
      );
    }
  }

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>
            {provider ? "Editar provedor" : "Novo provedor de modelo"}
          </DialogTitle>
          <DialogDescription>
            {t("integrations.credentialSealed")}
          </DialogDescription>
        </DialogHeader>

        <Form {...form}>
          <form
            onSubmit={form.handleSubmit(submit)}
            className="flex flex-col gap-4"
          >
            <FormField
              control={form.control}
              name="name"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t("admin.name")}</FormLabel>
                  <FormControl>
                    <Input
                      {...field}
                      disabled={!!provider}
                      className="font-mono"
                      placeholder="openai"
                    />
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
                      placeholder="https://api.openai.com/v1"
                    />
                  </FormControl>
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

            <DialogFooter>
              <Button type="button" variant="outline" onClick={onClose}>
                {t("common.cancel")}
              </Button>
              <Button type="submit" disabled={form.formState.isSubmitting}>
                {t("common.save")}
              </Button>
            </DialogFooter>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  );
}
