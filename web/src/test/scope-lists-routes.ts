import type { Company } from "@/features/companies/api";
import type { RegisteredScope } from "@/features/scope/api";

/**
 * The two lists the company and area selects read, as the server answers
 * them: every company with withdrawn ones marked, and the areas ordered by
 * company.
 */
export const COMPANIES: Company[] = [
  { id: "acme", label: "Acme", areas: 2, archived: false },
  { id: "example", label: "Example", areas: 1, archived: false },
  { id: "globex", label: "Globex", areas: 0, archived: true },
];

export const SCOPES: RegisteredScope[] = [
  { company: "acme", area: "finance", label: "Financeiro" },
  { company: "acme", area: "ops", label: "Operações" },
  { company: "example", area: "cx", label: "Atendimento" },
];

const json = (body: unknown) =>
  new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });

/**
 * Answers the two lists, or undefined for any other path. `fail` fails both;
 * `companies` replaces the company list, or refuses it as the server does a
 * caller without authority over the installation.
 */
export function scopeListsRoute(
  method: string,
  path: string,
  lists: { fail?: boolean; companies?: Company[] | "refused" } = {},
): Response | undefined {
  if (method !== "GET") return undefined;
  if (path !== "/admin/companies" && path !== "/admin/scopes") return undefined;
  if (lists.fail) return new Response(null, { status: 500 });
  if (path === "/admin/scopes") return json({ items: SCOPES });
  if (lists.companies === "refused") return new Response(null, { status: 403 });
  return json({ items: lists.companies ?? COMPANIES });
}

/** Where a fetch was aimed, whether it was given a Request or a string. */
export function pathOf(input: Request | string): string {
  const url = typeof input === "string" ? input : input.url;
  return new URL(url, "http://localhost").pathname.replace("/api/v1", "");
}
