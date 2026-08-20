import { StrictMode } from "react";
// Before anything renders: a component that called t() first would render key
// names for one pass and then correct itself.
import "@/i18n";
import { createRoot } from "react-dom/client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { BrowserRouter, Navigate, Route, Routes } from "react-router-dom";
import { Toaster } from "@/components/ui/sonner";
import { ThemeProvider } from "@/components/shared/theme-provider";
import { AppShell } from "@/components/layout/app-shell";
import { BrandingProvider } from "@/features/branding/branding-provider";
import { MoneyProvider } from "@/features/money/money-provider";
import { SessionGate } from "@/features/session/session-gate";
import { OverviewPage } from "@/features/overview/overview-page";
import { PoliciesPage } from "@/features/policies/policies-page";
import { PolicyEditorPage } from "@/features/policies/policy-editor-page";
import { AuditPage } from "@/features/audit/audit-page";
import { NotFoundPage } from "@/components/shared/not-found";
import { AgentsPage } from "@/features/agents/agents-page";
import { AgentDetailPage } from "@/features/agents/agent-detail-page";
import { InterviewPage } from "@/features/agents/interview-page";
import { AgentEditorPage } from "@/features/agents/agent-editor-page";
import { SimulationPage } from "@/features/agents/simulation-page";
import { RunsPage } from "@/features/runs/runs-page";
import { RunDetailPage } from "@/features/runs/run-detail-page";
import { RuntimePage } from "@/features/runtime/runtime-page";
import { ApprovalsPage } from "@/features/approvals/approvals-page";
import { CostPage } from "@/features/cost/cost-page";
import { ManualIndexPage } from "@/features/manual/manual-index-page";
import { ManualReadPage } from "@/features/manual/manual-page";
import { AdminPage } from "@/features/admin/admin-page";
import { IntegrationsPage } from "@/features/integrations/integrations-page";
import { MCPServerPage } from "@/features/integrations/mcp/mcp-server-page";
import { NewServerPage } from "@/features/integrations/mcp/new-server-page";
import { CataloguePage } from "@/features/integrations/mcp/catalogue-page";
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
        <BrandingProvider>
          <MoneyProvider>
            <BrowserRouter>
              <SessionGate>
                <AppShell>
                  <Routes>
                  <Route path="/" element={<Navigate to="/overview" replace />} />
                  <Route path="/overview" element={<OverviewPage />} />
                  <Route path="/agents" element={<AgentsPage />} />
                  <Route path="/agents/interview" element={<InterviewPage />} />
                  <Route path="/agents/new" element={<AgentEditorPage />} />
                  <Route path="/agents/:agentId" element={<AgentDetailPage />} />
                  <Route
                    path="/agents/:agentId/edit"
                    element={<AgentEditorPage />}
                  />
                  <Route
                    path="/agents/:agentId/simulate"
                    element={<SimulationPage />}
                  />
                  <Route path="/runs" element={<RunsPage />} />
                  <Route path="/runs/:runId" element={<RunDetailPage />} />
                  <Route path="/runtime" element={<RuntimePage />} />
                  <Route path="/approvals" element={<ApprovalsPage />} />
                  <Route path="/cost" element={<CostPage />} />
                  <Route path="/policies" element={<PoliciesPage />} />
                  <Route path="/policies/:code" element={<PolicyEditorPage />} />
                  <Route path="/audit" element={<AuditPage />} />
                  <Route path="/integrations" element={<IntegrationsPage />} />
                  <Route
                    path="/integrations/providers"
                    element={<IntegrationsPage section="providers" />}
                  />
                  <Route
                    path="/integrations/credentials"
                    element={<IntegrationsPage section="credentials" />}
                  />
                  <Route
                    path="/integrations/channels"
                    element={<IntegrationsPage section="channels" />}
                  />
                  <Route path="/integrations/mcp" element={<CataloguePage />} />
                  <Route
                    path="/integrations/mcp/new"
                    element={<NewServerPage />}
                  />
                  <Route
                    path="/integrations/mcp/:name"
                    element={<MCPServerPage />}
                  />
                  <Route path="/manual" element={<ManualIndexPage />} />
                  <Route path="/manual/:slug" element={<ManualReadPage />} />
                  <Route path="/admin" element={<AdminPage />} />
                  <Route path="*" element={<NotFoundPage />} />
                  </Routes>
                </AppShell>
              </SessionGate>
            </BrowserRouter>
          </MoneyProvider>
          <Toaster richColors closeButton />
        </BrandingProvider>
      </QueryClientProvider>
    </ThemeProvider>
  </StrictMode>,
);
