import { Trans, useTranslation } from "react-i18next";
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
import { usePutMCPServer, type MCPServer } from "@/features/integrations/api";

const schema = z.object({
  name: z
    .string()
    .min(1, "Dê um nome ao servidor.")
    .regex(/^[a-z0-9][a-z0-9_-]*$/, "integrations.nameCharset"),
  command: z.string().min(1, "Diga o que executar."),
  args: z.string(),
  enabled: z.boolean(),
});

export function ServerForm({
  server,
  onClose,
}: {
  server: MCPServer | null;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const put = usePutMCPServer();
  const form = useForm<z.infer<typeof schema>>({
    resolver: zodResolver(schema),
    defaultValues: {
      name: server?.name ?? "",
      command: server?.command ?? "",
      args: (server?.args ?? []).join(" "),
      enabled: server?.enabled ?? true,
    },
  });

  async function submit(values: z.infer<typeof schema>) {
    try {
      await put.mutateAsync({
        name: values.name,
        command: values.command,
        args: values.args.split(/\s+/).filter(Boolean),
        enabled: values.enabled,
      });
      toast.success(`${values.name} configurado`, {
        // Servers are connected when a worker starts, so saying it is live
        // would be a promise the platform does not keep yet.
        description:
          "As ferramentas aparecem quando um worker reiniciar e conectar.",
      });
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
            {server ? "Editar servidor" : "Novo servidor de ferramentas"}
          </DialogTitle>
          <DialogDescription>{t("integrations.mcpExplains")}</DialogDescription>
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
                      disabled={!!server}
                      className="font-mono"
                    />
                  </FormControl>
                  <FormDescription>
                    <Trans
                      i18nKey="integrations.prefixExplains"
                      components={{ code: <code className="font-mono" /> }}
                    />
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name="command"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t("integrations.command")}</FormLabel>
                  <FormControl>
                    <Input
                      {...field}
                      className="font-mono"
                      placeholder="/usr/local/bin/crm-mcp"
                    />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name="args"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t("integrations.arguments")}</FormLabel>
                  <FormControl>
                    <Input
                      {...field}
                      className="font-mono"
                      placeholder="--config /etc/crm.yaml"
                    />
                  </FormControl>
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
