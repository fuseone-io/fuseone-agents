import { useTranslation } from "react-i18next";
import { Link, useLocation } from "react-router-dom";
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from "@/components/ui/breadcrumb";
import { Separator } from "@/components/ui/separator";
import { SidebarTrigger } from "@/components/ui/sidebar";
import { PageActionsTarget } from "@/components/layout/page-actions";
import { LanguageToggle } from "@/components/shared/language-toggle";
import { ThemeToggle } from "@/components/shared/theme-toggle";
import { PAGE_TITLES, SUB_TITLES } from "@/components/layout/nav";

/**
 * 52px, with a rule along its bottom edge.
 *
 * The rule is what separates chrome from content now that the content is flush
 * rather than a floating panel. Removing the panel and adding this are one
 * change: something has to draw that line, and a border costs one pixel where
 * a card cost a whole level of elevation.
 */
export function AppHeader() {
  const { t } = useTranslation();
  const { pathname } = useLocation();

  return (
    <header className="flex h-[52px] shrink-0 items-center gap-2 border-b border-border px-6">
      {/* The name comes from here rather than from the primitive: the CLI
          file ships "Toggle Sidebar" in English, and aria-label wins over
          its sr-only text without our having to fork the file. */}
      <SidebarTrigger className="-ml-1" aria-label={t("shell.toggleSidebar")} />
      <Separator orientation="vertical" className="mr-1 !h-[18px]" />
      <Crumbs pathname={pathname} />
      <div className="flex-1" />
      {/* The screen's own primary action, portalled up from its PageHeader.
          One per screen: the prototype shows a fixed "New agent" here, which
          would offer to create an agent from the cost report. */}
      <PageActionsTarget />
      <LanguageToggle />
      <ThemeToggle />
    </header>
  );
}

function Crumbs({ pathname }: { pathname: string }) {
  const { t } = useTranslation();
  const [section = "runs", detail] = pathname.split("/").filter(Boolean);
  // The key, or the segment itself for a screen with no name registered.
  const key = PAGE_TITLES[section];
  const title = key ? t(key) : section;

  return (
    <Breadcrumb>
      <BreadcrumbList className="text-sm">
        <BreadcrumbItem>
          {detail ? (
            <BreadcrumbLink asChild>
              <Link to={`/${section}`}>{title}</Link>
            </BreadcrumbLink>
          ) : (
            <BreadcrumbPage>{title}</BreadcrumbPage>
          )}
        </BreadcrumbItem>
        {detail && (
          <>
            <BreadcrumbSeparator />
            <BreadcrumbItem>
              {/* A record's identifier is machine-generated and reads in
                  mono. A named sub-screen is not one: "interview" set in the
                  same face as a run id claims to be a record somebody could
                  look up. */}
              {SUB_TITLES[detail] ? (
                <BreadcrumbPage>{t(SUB_TITLES[detail])}</BreadcrumbPage>
              ) : (
                <BreadcrumbPage className="font-mono text-xs">
                  {detail}
                </BreadcrumbPage>
              )}
            </BreadcrumbItem>
          </>
        )}
      </BreadcrumbList>
    </Breadcrumb>
  );
}
