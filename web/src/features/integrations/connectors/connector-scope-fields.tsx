import { ConnectorFreeScopeFields } from "@/features/integrations/connectors/connector-free-scope-fields";
import { ConnectorHeldScopeField } from "@/features/integrations/connectors/connector-held-scope-field";
import { useContext } from "react";
import { ConnectorScopeContext } from "@/features/integrations/connectors/connector-scope-context";

/**
 * Where a connector instance lives (NT-019).
 *
 * Whoever governs every scope, the platform's administration, names it freely.
 * Anyone else picks one of the scopes where they configure, and a new
 * instance starts in the first of them: an area's administrator never types a
 * scope the server would refuse.
 */
export function ConnectorScopeFields({ editing }: { editing: boolean }) {
  const access = useContext(ConnectorScopeContext);
  return access.anyScope ? (
    <ConnectorFreeScopeFields editing={editing} />
  ) : (
    <ConnectorHeldScopeField editing={editing} scopes={access.configure} />
  );
}
