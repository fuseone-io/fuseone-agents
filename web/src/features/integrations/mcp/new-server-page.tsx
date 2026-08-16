import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router-dom";
import { Server } from "lucide-react";
import { PageHeader } from "@/components/shared/page-header";
import { Panel } from "@/components/shared/panel";
import { Recipes } from "@/features/integrations/mcp/recipes";
import { Button } from "@/components/ui/button";
import { Form } from "@/components/ui/form";
import { ServerFormBody } from "@/features/integrations/server-form-body";
import { useServerForm } from "@/features/integrations/mcp/use-server-form";

/**
 * Connecting a tool server, with the catalogue in front of it.
 *
 * A page rather than a dialog, and the dialog is why. A grid of cards has no
 * business in a modal — it drew over the fields underneath — but the deeper
 * reason is that a catalogue is read before anything is decided, and reading
 * needs somewhere to stand.
 *
 * The order is the order the decisions happen in: see what is known, fill the
 * form from one of them or not, and accept what a local server is. Nothing
 * here brings a tool in or says what one does; both are further along, on the
 * server's own page, and neither follows from connecting.
 */
export function NewServerPage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { form, submit, saving } = useServerForm(null, () =>
    void navigate("/integrations"),
  );

  return (
    <div className="space-y-6">
      <PageHeader
        icon={Server}
        title={t("mcp.connectAServer")}
        description={t("mcp.connectDescription")}
      />

      <Panel title={t("mcp.startFromARecipe")}>
        <Recipes form={form} />
      </Panel>

      <Panel title={t("mcp.connection")}>
        <Form {...form}>
          <form
            onSubmit={form.handleSubmit(submit)}
            className="flex flex-col gap-4"
          >
            <ServerFormBody form={form} editing={false} hasSecret={false} />
            <div className="flex justify-end gap-2">
              <Button
                type="button"
                variant="ghost"
                onClick={() => void navigate("/integrations")}
              >
                {t("common.cancel")}
              </Button>
              <Button type="submit" disabled={saving}>
                {t("integrations.connect")}
              </Button>
            </div>
          </form>
        </Form>
      </Panel>
    </div>
  );
}
