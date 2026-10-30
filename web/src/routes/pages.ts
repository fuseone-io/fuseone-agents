import { lazy } from "react";

// Every screen is its own chunk, fetched the first time somebody opens it.
//
// The console used to import all of them eagerly into one 2.1 MB bundle, so a
// visitor who only reached the sign-in screen downloaded the run diagram, the
// policy editor and the markdown renderer before seeing a password field.
// Screens export named components; lazy() wants a default, hence the `.then`.

export const OverviewPage = lazy(() =>
  import("@/features/overview/overview-page").then((m) => ({ default: m.OverviewPage })),
);
export const AgentsPage = lazy(() =>
  import("@/features/agents/agents-page").then((m) => ({ default: m.AgentsPage })),
);
export const InterviewPage = lazy(() =>
  import("@/features/agents/interview-page").then((m) => ({ default: m.InterviewPage })),
);
export const AgentEditorPage = lazy(() =>
  import("@/features/agents/agent-editor-page").then((m) => ({ default: m.AgentEditorPage })),
);
export const AgentDetailPage = lazy(() =>
  import("@/features/agents/agent-detail-page").then((m) => ({ default: m.AgentDetailPage })),
);
export const SimulationPage = lazy(() =>
  import("@/features/agents/simulation-page").then((m) => ({ default: m.SimulationPage })),
);
export const RunsPage = lazy(() =>
  import("@/features/runs/runs-page").then((m) => ({ default: m.RunsPage })),
);
export const RunDetailPage = lazy(() =>
  import("@/features/runs/run-detail-page").then((m) => ({ default: m.RunDetailPage })),
);
export const RuntimePage = lazy(() =>
  import("@/features/runtime/runtime-page").then((m) => ({ default: m.RuntimePage })),
);
export const ApprovalsPage = lazy(() =>
  import("@/features/approvals/approvals-page").then((m) => ({ default: m.ApprovalsPage })),
);
export const CostPage = lazy(() =>
  import("@/features/cost/cost-page").then((m) => ({ default: m.CostPage })),
);
export const MemoryPage = lazy(() =>
  import("@/features/memory/memory-page").then((m) => ({ default: m.MemoryPage })),
);
export const PoliciesPage = lazy(() =>
  import("@/features/policies/policies-page").then((m) => ({ default: m.PoliciesPage })),
);
export const PolicyEditorPage = lazy(() =>
  import("@/features/policies/policy-editor-page").then((m) => ({ default: m.PolicyEditorPage })),
);
export const AuditPage = lazy(() =>
  import("@/features/audit/audit-page").then((m) => ({ default: m.AuditPage })),
);
export const IntegrationsPage = lazy(() =>
  import("@/features/integrations/integrations-page").then((m) => ({ default: m.IntegrationsPage })),
);
export const CataloguePage = lazy(() =>
  import("@/features/integrations/mcp/catalogue-page").then((m) => ({ default: m.CataloguePage })),
);
export const NewServerPage = lazy(() =>
  import("@/features/integrations/mcp/new-server-page").then((m) => ({ default: m.NewServerPage })),
);
export const MCPServerPage = lazy(() =>
  import("@/features/integrations/mcp/mcp-server-page").then((m) => ({ default: m.MCPServerPage })),
);
export const ManualIndexPage = lazy(() =>
  import("@/features/manual/manual-index-page").then((m) => ({ default: m.ManualIndexPage })),
);
export const ManualReadPage = lazy(() =>
  import("@/features/manual/manual-page").then((m) => ({ default: m.ManualReadPage })),
);
export const AdminPage = lazy(() =>
  import("@/features/admin/admin-page").then((m) => ({ default: m.AdminPage })),
);
