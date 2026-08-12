import { useTranslation } from "react-i18next";
import { PAGE_ICONS } from "@/components/layout/nav";
import { PageHeader } from "@/components/shared/page-header";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { ToolsPanel } from "@/features/admin/tools-panel";
import { EventsPanel } from "@/features/admin/events-panel";
import { AuthoringPanel } from "@/features/admin/authoring-panel";
import { AreasPanel } from "@/features/admin/areas-panel";
import { BudgetsPanel } from "@/features/admin/budgets-panel";

/**
 * Everything an operator configures lives here, and every change made here is
 * recorded. That pairing is the point: the platform's rules are editable and
 * the edits are auditable, or neither is worth much.
 */
export function AdminPage() {
  const { t } = useTranslation();
  return (
    <>
      <PageHeader
        icon={PAGE_ICONS.admin}
        title={t("nav.admin")}
        description={t("admin.subtitle")}
      />

      <Tabs defaultValue="tools" className="min-h-0 flex-1">
        <TabsList>
          <TabsTrigger value="tools">{t("admin.tools")}</TabsTrigger>
          <TabsTrigger value="authoring">{t("admin.authoring")}</TabsTrigger>
          <TabsTrigger value="areas">{t("admin.areas")}</TabsTrigger>
          <TabsTrigger value="budgets">{t("admin.budgets")}</TabsTrigger>
          <TabsTrigger value="events">{t("admin.trail")}</TabsTrigger>
        </TabsList>

        <TabsContent value="tools" className="mt-4">
          <ToolsPanel />
        </TabsContent>
        <TabsContent value="authoring" className="mt-4">
          <AuthoringPanel />
        </TabsContent>
        <TabsContent value="areas" className="mt-4">
          <AreasPanel />
        </TabsContent>
        <TabsContent value="budgets" className="mt-4">
          <BudgetsPanel />
        </TabsContent>
        <TabsContent value="events" className="mt-4">
          <EventsPanel />
        </TabsContent>
      </Tabs>
    </>
  );
}
