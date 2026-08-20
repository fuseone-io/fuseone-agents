import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import {
  PropertiesSheet,
  PropertiesSheetBody,
  PropertiesSheetFooter,
} from "@/components/shared/properties-sheet";
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
    <PropertiesSheet
      open
      onOpenChange={(open) => !open && onClose()}
      title={
        server ? t("integrations.editServer") : t("integrations.newServer")
      }
      description={t("integrations.mcpExplains")}
    >
      <Form {...form}>
        <form
          onSubmit={form.handleSubmit(submit)}
          className="flex min-h-0 flex-1 flex-col"
        >
          <PropertiesSheetBody
            data-testid="server-form-scroll"
            className="space-y-4"
          >
              <ServerFormBody
                form={form}
                editing={server !== null}
                hasSecret={server?.hasSecret === true}
                hasConfigFile={server?.hasConfigFile === true}
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
