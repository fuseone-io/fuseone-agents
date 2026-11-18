import { Navigate, useParams } from "react-router-dom";

/**
 * /agents/:agentId/edit, where the form used to be: an agent is edited in
 * the workbench now (NT-018, delivery R), and a link already shared lands
 * there.
 */
export function AgentEditRedirect() {
  const { agentId = "" } = useParams();
  return <Navigate to={`/agents/${agentId}/workbench`} replace />;
}
