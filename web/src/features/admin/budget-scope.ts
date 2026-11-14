import type { ScopeBudget } from "@/features/admin/api";
import type { Scope } from "@/features/scope/active-scope";

/** The path form the API takes: installation, a company, or company/area. */
export function scopePath(budget: ScopeBudget): string {
  const company = budget.scope?.company ?? "";
  const area = budget.scope?.area ?? "";
  if (!company) return "installation";
  return area ? `${company}/${area}` : company;
}

const INSTALLATION = "*";

/**
 * The company and area selects' value for a path: the installation is the
 * company `*`, which is how grants name it too.
 */
export function scopeOfPath(path: string): Scope {
  if (path === "") return { company: "", area: "" };
  if (path === "installation") return { company: INSTALLATION, area: "" };
  const [company = "", area = ""] = path.split("/");
  return { company, area };
}

/**
 * The path for the selects' value, as the form has always sent it. Each part
 * loses its own outer spaces before they are joined: typed in free mode,
 * " acme " and " cx " are acme/cx, which the server would otherwise take as
 * another scope.
 */
export function pathOfScope(scope: Scope): string {
  const company = scope.company.trim();
  const area = scope.area.trim();
  if (company === "") return "";
  if (company === INSTALLATION) return "installation";
  return area ? `${company}/${area}` : company;
}

/** How a scope reads to a person. */
export function scopeLabel(budget: ScopeBudget): string {
  const path = scopePath(budget);
  return path === "installation" ? "admin.wholeInstallation" : path;
}
