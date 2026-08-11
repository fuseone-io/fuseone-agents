import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { BrowserRouter, Navigate, Route, Routes } from "react-router-dom";
import { Toaster } from "@/components/ui/sonner";
import { ThemeProvider } from "@/components/shared/theme-provider";
import { AppShell } from "@/components/layout/app-shell";
import { SessionGate } from "@/features/session/session-gate";
import { OverviewPage } from "@/features/overview/overview-page";
import { AuditPage } from "@/features/audit/audit-page";
import { NotFoundPage } from "@/components/shared/not-found";
import { AgentsPage } from "@/features/agents/agents-page";
import { AgentDetailPage } from "@/features/agents/agent-detail-page";
import { RunsPage } from "@/features/runs/runs-page";
import { RunDetailPage } from "@/features/runs/run-detail-page";
import { ApprovalsPage } from "@/features/approvals/approvals-page";
import { CostPage } from "@/features/cost/cost-page";
import { AdminPage } from "@/features/admin/admin-page";
import "./index.css";

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
        <BrowserRouter>
          <SessionGate>
            <AppShell>
              <Routes>
                <Route path="/" element={<Navigate to="/overview" replace />} />
                <Route path="/overview" element={<OverviewPage />} />
                <Route path="/agents" element={<AgentsPage />} />
                <Route path="/agents/:agentId" element={<AgentDetailPage />} />
              <Route path="/runs" element={<RunsPage />} />
                <Route path="/runs/:runId" element={<RunDetailPage />} />
                <Route path="/approvals" element={<ApprovalsPage />} />
                <Route path="/cost" element={<CostPage />} />
              <Route path="/audit" element={<AuditPage />} />
                <Route path="/admin" element={<AdminPage />} />
                <Route path="*" element={<NotFoundPage />} />
              </Routes>
            </AppShell>
          </SessionGate>
        </BrowserRouter>
        <Toaster richColors closeButton />
      </QueryClientProvider>
    </ThemeProvider>
  </StrictMode>,
);
