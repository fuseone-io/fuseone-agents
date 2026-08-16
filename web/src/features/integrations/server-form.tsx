import { useTranslation } from "react-i18next";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Form } from "@/components/ui/form";
import { ServerFormBody } from "@/features/integrations/server-form-body";
import {
  serverSchema,
  type ServerFormValues,
} from "@/features/integrations/server-schema";
import { usePutMCPServer, type MCPServer } from "@/features/integrations/api";
import { problemMessage } from "@/lib/api/problem-message";

export function ServerForm({
  server,
  onClose,
}: {
  server: MCPServer | null;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const put = usePutMCPServer();
  const form = useForm<ServerFormValues>({
    resolver: zodResolver(serverSchema),
    defaultValues: {
      name: server?.name ?? "",
      transport: server?.transport ?? "stdio",
      command: server?.command ?? "",
      args: (server?.args ?? []).join(" "),
      url: server?.url ?? "",
      token: "",
      // Never carried forward from the transport. A server nobody has
      // accepted must show as not accepted, or the box would tick itself on
      // the screen where the decision is supposed to be made.
      acceptsLocalExecution: server?.acceptsLocalExecution ?? false,
      enabled: server?.enabled ?? true,
    },
  });

  async function submit(values: ServerFormValues) {
    try {
      await put.mutateAsync({
        name: values.name,
        transport: values.transport,
        command: values.command,
        args: values.args.split(/\s+/).filter(Boolean),
        url: values.url,
        token: values.token,
        acceptsLocalExecution: values.acceptsLocalExecution,
        enabled: values.enabled,
      });
      toast.success(t("integrations.serverConfigured", { name: values.name }), {
        // A worker picks the change up on its next pass rather than at its
        // next restart, which is a wait with an end somebody can be told.
        description: t("integrations.toolsAppearHint"),
      });
      onClose();
    } catch (error) {
      toast.error(
        problemMessage(error, t),
      );
    }
  }

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>
            {server
              ? t("integrations.editServer")
              : t("integrations.newServer")}
          </DialogTitle>
          <DialogDescription>{t("integrations.mcpExplains")}</DialogDescription>
        </DialogHeader>

        <Form {...form}>
          <form
            onSubmit={form.handleSubmit(submit)}
            className="flex flex-col gap-4"
          >
            <ServerFormBody
              form={form}
              editing={server !== null}
              hasSecret={server?.hasSecret === true}
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
