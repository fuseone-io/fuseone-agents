import type { Me, MeScope } from "@/features/session/api";
import type { components } from "@/lib/api/schema.gen";

type Area = { company: string; area: string; label?: string };
type Connection = Pick<components["schemas"]["Channel"], "scopeKind" | "company" | "area">;

/** The company a grant names to hold the installation itself. */
const INSTALLATION_COMPANY = "*";

/**
 * Which scopes a conversation on a connection is offered (NT-019, phase 4).
 *
 * Only those the connection carries, since the server refuses a conversation
 * outside its connection's scope, and only those the caller configures, since
 * it refuses the rest too. The room for the whole installation only on the
 * installation's connection, and only to whoever governs the installation: a
 * company:write held over one company is not that authority, and offering it
 * would be a control that answers 403. Null is a session with no restrictions.
 */
export function offeredScopes<A extends Area>(
  areas: A[],
  connection: Connection | undefined,
  me: Me | null | undefined,
): { installation: boolean; scopes: A[] } {
  const atInstallation = !connection?.scopeKind || connection.scopeKind === "installation";
  return {
    installation: atInstallation && governsInstallation(me),
    scopes: areas.filter((a) => carried(a, connection) && configures(a, me)),
  };
}

function carried(a: Area, c: Connection | undefined) {
  if (!c?.scopeKind || c.scopeKind === "installation") return true;
  if (a.company !== c.company) return false;
  return c.scopeKind === "company" || a.area === c.area;
}

function configures(a: Area, me: Me | null | undefined) {
  // Whoever configures at the platform's administration configures every
  // area; a server that predates configure says nothing, and nothing is
  // narrowed on its behalf.
  if (!me || !me.configure || me.platform?.includes("provider:write")) return true;
  return (me.configure ?? []).some((held: MeScope) => held.company === a.company && (!held.area || held.area === a.area));
}

function governsInstallation(me: Me | null | undefined) {
  if (me === null) return true;
  if (!me?.can.includes("company:write")) return false;
  return me.grants.some((g) => g.company === INSTALLATION_COMPANY && g.area === "");
}

/**
 * Whether the caller configures a conversation already stored at scope
 * (NT-019, phase 4): its edit, test and removal are offered to them only then.
 * On a connection an area inherits, the company's room and the installation's
 * are read there, and every action on them would answer 403. The installation's
 * room is its governor's alone. Null and a session that predates configure
 * are not narrowed.
 */
export function configuresConversation(
  me: Me | null | undefined,
  scope: { company: string; area?: string },
): boolean {
  if (scope.company === INSTALLATION_COMPANY) {
    return me === null || me === undefined || (governsInstallation(me) && Boolean(me.platform?.includes("provider:write")));
  }
  return configures({ company: scope.company, area: scope.area ?? "" }, me);
}
