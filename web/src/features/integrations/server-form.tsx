import { useState } from "react";
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
import { useServerRecipe } from "@/features/integrations/mcp/use-server-recipe";
import { type MCPServer } from "@/features/integrations/api";
import { StaleNotice } from "@/components/shared/stale-notice";
import type { FormRevision } from "@/lib/api/revision";

export function ServerForm({
  server,
  onClose,
  revision,
}: {
  server: MCPServer | null;
  onClose: () => void;
  /** What the editor read; a server being connected has none. */
  revision?: FormRevision;
}) {
  const { t } = useTranslation();
  const [stale, setStale] = useState(false);
  // A server being connected here has no recipe; one stored is described by
  // the recipe of its name, if any, so the form offers what that recipe says.
  const recipe = useServerRecipe(server?.name);
  const { form, submit } = useServerForm(
    server,
    onClose,
    recipe,
    revision && { etag: revision.etag, onStale: () => setStale(true) },
  );

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
              {stale && revision && (
                <StaleNotice onReload={revision.reload} reloading={revision.reloading} />
              )}
              <ServerFormBody
                form={form}
                editing={server !== null}
                hasSecret={server?.hasSecret === true}
                hasConfigFile={server?.hasConfigFile === true}
                recipe={recipe}
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
