import { useTranslation } from "react-i18next";
import type { ComponentType } from "react";
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
import {
  type AdminTabValue,
  visibleAdminTabs,
} from "@/features/admin/admin-tabs";
import { useMe } from "@/features/session/api";

const ADMIN_PANELS = {
  tools: ToolsPanel,
  branding: BrandingPanel,
  authoring: AuthoringPanel,
  companies: CompaniesPanel,
  areas: AreasPanel,
  identity: IdentityPanel,
  people: PeoplePanel,
  prices: PricesPanel,
  budgets: BudgetsPanel,
  retention: RetentionPanel,
  events: EventsPanel,
} satisfies Record<AdminTabValue, ComponentType>;

/**
 * Everything an operator configures lives here, and every change made here is
 * recorded. That pairing is the point: the platform's rules are editable and
 * the edits are auditable, or neither is worth much.
 */
export function AdminPage() {
  const { t } = useTranslation();
  const { data: me } = useMe();
  const tab = useTab("admin", "tools");
  const can = me === null ? null : me?.can;
  const visibleTabs = visibleAdminTabs(can);
  const value = visibleTabs.some((item) => item.value === tab.value)
    ? tab.value
    : visibleTabs[0]?.value;

  return (
    <>
      <PageHeader
        icon={PAGE_ICONS.admin}
        title={t("nav.admin")}
        description={t("admin.subtitle")}
      />

      {visibleTabs.length === 0 ? (
        <div className="rounded-card border bg-card p-6 text-sm text-muted-foreground shadow-sm">
          {t("admin.noAvailableSections")}
        </div>
      ) : (
        /* Vertical, because nine tabs in a row is a row that wraps on a
          laptop and reads as a paragraph of links rather than as navigation.
          Down the side they are a list, they have room for their full names,
          and the one in force is obvious without counting. */
        <Tabs
          value={value}
          onValueChange={tab.onValueChange}
          orientation="vertical"
          className="min-h-0 flex-1 flex-col gap-6 lg:flex-row"
        >
          <TabsList className="w-full shrink-0 self-stretch lg:w-48 lg:self-start">
            {visibleTabs.map((item) => (
              <TabsTrigger key={item.value} value={item.value}>
                {t(item.label)}
              </TabsTrigger>
            ))}
          </TabsList>

          {visibleTabs.map((item) => {
            const Panel = ADMIN_PANELS[item.value];
            return (
              <TabsContent key={item.value} value={item.value} className="min-w-0">
                <Panel />
              </TabsContent>
            );
          })}
        </Tabs>
      )}
    </>
  );
}
