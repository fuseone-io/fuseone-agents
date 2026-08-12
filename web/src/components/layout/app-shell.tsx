import type { ReactNode } from "react";
import { SidebarInset, SidebarProvider } from "@/components/ui/sidebar";
import { AppSidebar } from "@/components/layout/app-sidebar";
import { AppHeader } from "@/components/layout/app-header";

/**
 * The sidebar-07 shell: an icon-collapsible sidebar and a flush content area.
 *
 * The content is not a card. It used to be — a panel floating on the sidebar
 * colour — and the cost was two levels of lift: the page rose off the ground,
 * and every card inside rose off the page. Elevation belongs to the cards, and
 * a container that also lifts makes the ones inside it mean less.
 *
 * With the panel gone, something has to separate chrome from content, and that
 * is the header's bottom rule. The two changes are one change: the rule exists
 * because the card no longer does.
 */
export function AppShell({ children }: { children: ReactNode }) {
  return (
    <SidebarProvider className="bg-background">
      <AppSidebar />

      <SidebarInset className="min-w-0 bg-background">
        <AppHeader />
        {/* SidebarInset is already the page's main landmark. */}
        <div className="flex min-h-0 flex-1 flex-col gap-6 overflow-auto p-6">
          {children}
        </div>
      </SidebarInset>
    </SidebarProvider>
  );
}
