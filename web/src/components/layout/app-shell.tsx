import type { ReactNode } from "react";
import { SidebarInset, SidebarProvider } from "@/components/ui/sidebar";
import { AppSidebar } from "@/components/layout/app-sidebar";
import { AppHeader } from "@/components/layout/app-header";

/**
 * The sidebar-07 shell: an icon-collapsible sidebar and an inset content area.
 *
 * The whole page sits on the sidebar colour and the content is a card floating
 * on it. That is what makes the working area read as lifted without a heavy
 * border, and it is why the header carries no bottom rule — the card's own
 * edge already separates chrome from content.
 */
export function AppShell({ children }: { children: ReactNode }) {
  return (
    <SidebarProvider className="bg-sidebar">
      <AppSidebar />

      <SidebarInset className="min-w-0 bg-transparent">
        <AppHeader />
        {/* SidebarInset is already the page's main landmark; the card is the
            surface, not a second one. */}
        <div className="mx-4 mb-4 flex min-h-0 flex-1 flex-col gap-6 overflow-auto rounded-2xl border bg-background p-10 shadow-sm">
          {children}
        </div>
      </SidebarInset>
    </SidebarProvider>
  );
}
