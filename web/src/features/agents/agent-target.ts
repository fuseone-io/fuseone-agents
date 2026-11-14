/**
 * The target scope an author picks tools and providers for (NT-019, rule 5):
 * `company/area`, `company` alone, or nothing while neither is chosen.
 *
 * Nothing is today's answer for a new agent without an area: the platform's
 * reader is answered everything and anyone else the installation's, and the
 * picker narrows to the agent's own reach the moment an area is chosen.
 */
export function agentTarget(scope: {
  company: string;
  area?: string;
}): string | undefined {
  const company = scope.company.trim();
  const area = (scope.area ?? "").trim();
  if (company === "") return undefined;
  return area === "" ? company : `${company}/${area}`;
}
