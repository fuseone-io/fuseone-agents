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
import { ThemeToggle } from "@/components/shared/theme-toggle";
import { PAGE_TITLES } from "@/components/layout/nav";

/**
 * 52px, transparent, no bottom border — it sits on the sidebar-coloured
 * ground alongside the content card rather than capping it.
 */
export function AppHeader() {
  const { pathname } = useLocation();

  return (
    <header className="flex h-[52px] shrink-0 items-center gap-2 px-4">
      <SidebarTrigger className="-ml-1" />
      <Separator orientation="vertical" className="mr-1 !h-[18px]" />
      <Crumbs pathname={pathname} />
      <div className="flex-1" />
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
