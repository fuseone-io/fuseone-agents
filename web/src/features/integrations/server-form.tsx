import { useTranslation } from "react-i18next";
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
import { useServerForm } from "@/features/integrations/mcp/use-server-form";
import { type MCPServer } from "@/features/integrations/api";

export function ServerForm({
  server,
  onClose,
}: {
  server: MCPServer | null;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const { form, submit } = useServerForm(server, onClose);

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
              hasConfigFile={server?.hasConfigFile === true}
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
