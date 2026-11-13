import { useContext } from "react";
import { FreeScopeFields } from "@/features/integrations/scope/free-scope-fields";
import { HeldScopeField } from "@/features/integrations/scope/held-scope-field";
import { ScopeChoiceContext } from "@/features/integrations/scope/scope-choice";

/**
 * Where a connector instance or an MCP server lives (NT-019).
 *
 * Whoever governs every scope, the platform's administration, names it freely.
 * Anyone else picks one of the scopes where they configure, and a new row
 * starts in the first of them: an area's administrator never types a scope the
 * server would refuse.
 */
export function ScopeFields({
  editing,
  labels,
}: {
  editing: boolean;
  /** i18n keys: the free kind's label, and the held scope's. */
  labels: { kind: string; held: string };
}) {
  const access = useContext(ScopeChoiceContext);
  return access.anyScope ? (
    <FreeScopeFields editing={editing} label={labels.kind} />
  ) : (
    <HeldScopeField editing={editing} scopes={access.configure} label={labels.held} />
  );
}
