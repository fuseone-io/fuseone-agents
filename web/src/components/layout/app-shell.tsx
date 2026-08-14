import type { ReactNode } from "react";
import { SidebarInset, SidebarProvider } from "@/components/ui/sidebar";
import { AppSidebar } from "@/components/layout/app-sidebar";
import { AppHeader } from "@/components/layout/app-header";
import {
  PageActionsProvider,
  useShellChrome,
} from "@/components/layout/page-actions";
import { StopBanner } from "@/features/admin/stop-banner";
import { usePreferences } from "@/features/preferences/use-preferences";
import { cn } from "@/lib/utils";

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
    <PageActionsProvider>
      <Shell>{children}</Shell>
    </PageActionsProvider>
  );
}

/**
 * The shell proper, inside the slots so it can read what the screen asked for.
 *
 * A screen that wants the navigation out of the way asks; it does not write to
 * the stored preference. Borrowing a setting and forgetting to give it back is
 * how somebody finds their sidebar collapsed on a screen that never mentioned
 * it — and stops trusting the rest of their preferences.
 */
function Shell({ children }: { children: ReactNode }) {
  const sidebarOpen = usePreferences((state) => state.sidebarOpen);
  const setSidebarOpen = usePreferences((state) => state.setSidebarOpen);
  const { compact } = useShellChrome();

  return (
    // h-svh, not the block's min-h-svh. With only a minimum the wrapper grew
    // with the page, the content area below never had a height to overflow,
    // and so the whole document scrolled: the header's rule and its action
    // scrolled away with it, and every sticky element inside was inert
    // because its scroll container never scrolled.
    // Controlled, so the choice survives a reload. The block ships this
    // uncontrolled, which means somebody who works with the sidebar collapsed
    // collapses it again on every single page load.
    <SidebarProvider
      className="h-svh bg-background"
      open={compact ? false : sidebarOpen}
      onOpenChange={setSidebarOpen}
    >
      <AppSidebar />

      <SidebarInset className="min-w-0 bg-background">
        {/* Wraps both, so a page deep in the content can render its action
            into the header above it. */}
        <>
          <AppHeader />
          {/* SidebarInset is already the page's main landmark.
              Children do not shrink. A card that clips its own corners with
              overflow-hidden loses its automatic minimum size, so in a column
              with a real height it collapses instead of overflowing — the runs
              table came out 197px tall over 2078px of rows, with no scrollbar
              to say so. A page that wants the full height asks with flex-1,
              which is sized from zero and unaffected by this. */}
          {/* A compact screen brings its own frame: the padding and the gap
              here would sit outside a header it draws itself, and the height
              it computes would be wrong by exactly that padding — which is
              how a footer ends up over the last row of a list. */}
          <div
            className={cn(
              "flex min-h-0 flex-1 flex-col overflow-auto [&>*]:shrink-0",
              !compact && "gap-6 p-6",
            )}
          >
            {/* Above every screen's content, not inside one panel. The
                question it answers is "why is nothing running", and somebody
                asking that is looking at stale numbers, not at the
                administration area (PRD FO-06). */}
            <StopBanner />
            {children}
          </div>
        </>
      </SidebarInset>
    </SidebarProvider>
  );
}
