import type { ScopeBudget } from "@/features/admin/api";

/** The path form the API takes: installation, a company, or company/area. */
export function scopePath(budget: ScopeBudget): string {
  const company = budget.scope?.company ?? "";
  const area = budget.scope?.area ?? "";
  if (!company) return "installation";
  return area ? `${company}/${area}` : company;
}

/** How a scope reads to a person. */
export function scopeLabel(budget: ScopeBudget): string {
  const path = scopePath(budget);
  return path === "installation" ? "Instalação inteira" : path;
}
