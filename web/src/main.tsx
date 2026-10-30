import { lazy, StrictMode, Suspense } from "react";
// Before anything renders: a component that called t() first would render key
// names for one pass and then correct itself.
import "@/i18n";
import { createRoot } from "react-dom/client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { BrowserRouter } from "react-router-dom";
import { Toaster } from "@/components/ui/sonner";
import { ThemeProvider } from "@/components/shared/theme-provider";
import { BrandingProvider } from "@/features/branding/branding-provider";
import { MoneyProvider } from "@/features/money/money-provider";
import { SessionGate } from "@/features/session/session-gate";
import { ShellFallback } from "@/routes/shell-fallback";
import "./index.css";

// The console is fetched only once somebody is past the session gate: the
// sign-in and setup screens are all an anonymous visitor ever downloads.
const ConsoleRoutes = lazy(() => import("@/routes/console-routes"));

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 5_000,
      // A failed read is shown with a retry the reader controls, rather than
      // the app silently hammering an API that is already unhappy.
      retry: 1,
      refetchOnWindowFocus: false,
    },
  },
});

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <ThemeProvider>
      <QueryClientProvider client={queryClient}>
        <BrandingProvider>
          <MoneyProvider>
            <BrowserRouter>
              <SessionGate>
                <Suspense fallback={<ShellFallback />}>
                  <ConsoleRoutes />
                </Suspense>
              </SessionGate>
            </BrowserRouter>
          </MoneyProvider>
          <Toaster richColors closeButton />
        </BrandingProvider>
      </QueryClientProvider>
    </ThemeProvider>
  </StrictMode>,
);
