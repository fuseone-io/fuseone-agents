import { useTranslation } from "react-i18next";
import { PAGE_ICONS } from "@/components/layout/nav";
import { PageHeader } from "@/components/shared/page-header";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { CompaniesPanel } from "@/features/companies/companies-panel";
import { useTab } from "@/features/preferences/use-preferences";
import { ToolsPanel } from "@/features/admin/tools-panel";
import { EventsPanel } from "@/features/admin/events-panel";
import { AuthoringPanel } from "@/features/admin/authoring-panel";
import { PricesPanel } from "@/features/admin/prices-panel";
import { IdentityPanel } from "@/features/admin/identity-panel";
import { PeoplePanel } from "@/features/admin/people-panel";
import { RetentionPanel } from "@/features/admin/retention-panel";
import { AreasPanel } from "@/features/admin/areas-panel";
import { BudgetsPanel } from "@/features/admin/budgets-panel";
import { BrandingPanel } from "@/features/admin/branding-panel";

/**
 * Everything an operator configures lives here, and every change made here is
 * recorded. That pairing is the point: the platform's rules are editable and
 * the edits are auditable, or neither is worth much.
 */
export function AdminPage() {
  const { t } = useTranslation();
  const tab = useTab("admin", "tools");

  return (
    <>
      <PageHeader
        icon={PAGE_ICONS.admin}
        title={t("nav.admin")}
        description={t("admin.subtitle")}
      />

      {/* Vertical, because nine tabs in a row is a row that wraps on a
          laptop and reads as a paragraph of links rather than as navigation.
          Down the side they are a list, they have room for their full names,
          and the one in force is obvious without counting. */}
      <Tabs
        {...tab}
        orientation="vertical"
        className="min-h-0 flex-1 flex-col gap-6 lg:flex-row"
      >
        <TabsList className="w-full shrink-0 self-stretch lg:w-48 lg:self-start">
          <TabsTrigger value="tools">{t("admin.toolsWaiting")}</TabsTrigger>
          <TabsTrigger value="branding">{t("admin.branding")}</TabsTrigger>
          <TabsTrigger value="authoring">{t("admin.authoring")}</TabsTrigger>
          <TabsTrigger value="companies">
            {t("companies.companies")}
          </TabsTrigger>
          <TabsTrigger value="areas">{t("admin.areas")}</TabsTrigger>
          <TabsTrigger value="identity">{t("admin.identity")}</TabsTrigger>
          <TabsTrigger value="people">{t("admin.people")}</TabsTrigger>
          <TabsTrigger value="prices">{t("admin.prices")}</TabsTrigger>
          <TabsTrigger value="budgets">{t("admin.budgets")}</TabsTrigger>
          <TabsTrigger value="retention">{t("admin.retention")}</TabsTrigger>
          <TabsTrigger value="events">{t("admin.trail")}</TabsTrigger>
        </TabsList>

        <TabsContent value="tools" className="min-w-0">
          <ToolsPanel />
        </TabsContent>
        <TabsContent value="branding" className="min-w-0">
          <BrandingPanel />
        </TabsContent>
        <TabsContent value="authoring" className="min-w-0">
          <AuthoringPanel />
        </TabsContent>
        <TabsContent value="identity" className="min-w-0">
          <IdentityPanel />
        </TabsContent>
        <TabsContent value="people" className="min-w-0">
          <PeoplePanel />
        </TabsContent>
        <TabsContent value="companies" className="min-w-0">
          <CompaniesPanel />
        </TabsContent>

        <TabsContent value="areas" className="min-w-0">
          <AreasPanel />
        </TabsContent>
        <TabsContent value="prices" className="min-w-0">
          <PricesPanel />
        </TabsContent>
        <TabsContent value="budgets" className="min-w-0">
          <BudgetsPanel />
        </TabsContent>
        <TabsContent value="retention" className="min-w-0">
          <RetentionPanel />
        </TabsContent>
        <TabsContent value="events" className="min-w-0">
          <EventsPanel />
        </TabsContent>
      </Tabs>
    </>
  );
}
