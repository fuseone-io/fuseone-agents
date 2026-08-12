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
import { ThemeToggle } from "@/components/shared/theme-toggle";
import { PAGE_TITLES } from "@/components/layout/nav";

/**
 * 52px, with a rule along its bottom edge.
 *
 * The rule is what separates chrome from content now that the content is flush
 * rather than a floating panel. Removing the panel and adding this are one
 * change: something has to draw that line, and a border costs one pixel where
 * a card cost a whole level of elevation.
 */
export function AppHeader() {
  const { pathname } = useLocation();

  return (
    <header className="flex h-[52px] shrink-0 items-center gap-2 border-b border-border px-6">
      <SidebarTrigger className="-ml-1" />
      <Separator orientation="vertical" className="mr-1 !h-[18px]" />
      <Crumbs pathname={pathname} />
      <div className="flex-1" />
      {/* The screen's own primary action, portalled up from its PageHeader.
          One per screen: the prototype shows a fixed "New agent" here, which
          would offer to create an agent from the cost report. */}
      <PageActionsTarget />
      <ThemeToggle />
    </header>
  );
}

function Crumbs({ pathname }: { pathname: string }) {
  const [section = "runs", detail] = pathname.split("/").filter(Boolean);
  const title = PAGE_TITLES[section] ?? section;

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
              {/* A run id is machine-generated, so it reads in mono. */}
              <BreadcrumbPage className="font-mono text-xs">{detail}</BreadcrumbPage>
            </BreadcrumbItem>
          </>
        )}
      </BreadcrumbList>
    </Breadcrumb>
  );
}
