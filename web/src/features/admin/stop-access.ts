import type { RegisteredScope } from "@/features/scope/api";
import type { Me, MeGrant } from "@/features/session/api";

export const EVERYTHING = "__installation__";

export interface StopTarget {
  value: string;
  company?: string;
  area?: string;
  label?: string;
}

const INSTALLATION = "*";
const RUN_READ_ROLES = new Set(["author", "approver", "auditor", "curator"]);

export function stopTargetsFor(
  me: Me | null,
  scopes: RegisteredScope[],
): StopTarget[] {
  const targets: StopTarget[] = [];
  const unrestricted = me === null;
  const grants = me?.grants ?? [];

  if (unrestricted || grants.some(canStopInstallation)) {
    targets.push({ value: EVERYTHING });
  }

  for (const scope of scopes) {
    if (unrestricted || grants.some((grant) => canStopScope(grant, scope))) {
      targets.push({
        value: `${scope.company}/${scope.area}`,
        company: scope.company,
        area: scope.area,
        label: scope.label || scope.area,
      });
    }
  }

  return targets;
}

function canStopInstallation(grant: MeGrant): boolean {
  return canReadRuns(grant) && grant.company === INSTALLATION && grant.area === "";
}

function canStopScope(grant: MeGrant, scope: RegisteredScope): boolean {
  if (!canReadRuns(grant)) return false;
  if (grant.company === INSTALLATION) return true;
  if (grant.company !== scope.company) return false;
  return grant.area === "" || grant.area === scope.area;
}

// A UI hint mirroring the built-in roles. The server remains the authority.
function canReadRuns(grant: MeGrant): boolean {
  return RUN_READ_ROLES.has(grant.role);
}
