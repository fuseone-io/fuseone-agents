import { Suspense } from "react";
import { Navigate, Route, Routes } from "react-router-dom";
import { AppShell } from "@/components/layout/app-shell";
import { NotFoundPage } from "@/components/shared/not-found";
import { PageFallback } from "@/routes/page-fallback";
import * as P from "@/routes/pages";

/**
 * The signed-in console: the shell and every screen behind it.
 *
 * This module is itself loaded lazily by main.tsx, so the sign-in and setup
 * screens never pay for the shell. Inside it, each screen is a further chunk;
 * the Suspense boundary sits inside the shell so the sidebar and header stay
 * put while a screen's code arrives. It is deliberately not keyed by path:
 * React Router navigates inside a transition, so moving between screens keeps
 * the current one on display until the next is ready, and a param change
 * (/agents/a to /agents/b) does not remount the screen and lose its state.
 */
export default function ConsoleRoutes() {
  return (
    <AppShell>
      <Suspense fallback={<PageFallback />}>
        <Routes>
          <Route path="/" element={<Navigate to="/overview" replace />} />
          <Route path="/overview" element={<P.OverviewPage />} />
          <Route path="/agents" element={<P.AgentsPage />} />
          <Route path="/agents/interview" element={<P.InterviewPage />} />
          <Route path="/agents/new" element={<P.AgentEditorPage />} />
          <Route path="/agents/:agentId" element={<P.AgentDetailPage />} />
          <Route path="/agents/:agentId/edit" element={<P.AgentEditorPage />} />
          <Route path="/agents/:agentId/simulate" element={<P.SimulationPage />} />
          <Route path="/runs" element={<P.RunsPage />} />
          <Route path="/runs/:runId" element={<P.RunDetailPage />} />
          <Route path="/runtime" element={<P.RuntimePage />} />
          <Route path="/approvals" element={<P.ApprovalsPage />} />
          <Route path="/cost" element={<P.CostPage />} />
          <Route path="/memory" element={<P.MemoryPage />} />
          <Route path="/policies" element={<P.PoliciesPage />} />
          <Route path="/policies/:code" element={<P.PolicyEditorPage />} />
          <Route path="/audit" element={<P.AuditPage />} />
          <Route path="/integrations" element={<P.IntegrationsPage />} />
          <Route
            path="/integrations/providers"
            element={<P.IntegrationsPage section="providers" />}
          />
          <Route
            path="/integrations/credentials"
            element={<P.IntegrationsPage section="credentials" />}
          />
          <Route
            path="/integrations/connectors"
            element={<P.IntegrationsPage section="connectors" />}
          />
          <Route
            path="/integrations/channels"
            element={<P.IntegrationsPage section="channels" />}
          />
          <Route path="/integrations/mcp" element={<P.CataloguePage />} />
          <Route path="/integrations/mcp/new" element={<P.NewServerPage />} />
          <Route path="/integrations/mcp/:name" element={<P.MCPServerPage />} />
          <Route path="/manual" element={<P.ManualIndexPage />} />
          <Route path="/manual/:slug" element={<P.ManualReadPage />} />
          <Route path="/admin" element={<P.AdminPage />} />
          <Route path="*" element={<NotFoundPage />} />
        </Routes>
      </Suspense>
    </AppShell>
  );
}
