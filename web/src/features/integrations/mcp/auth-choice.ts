import {
  headerNames,
  type RemoteAuthPlan,
} from "@/features/integrations/mcp/auth-plan";

/**
 * One way a remote connection authenticates, chosen before anything is typed.
 *
 * The platform stores one shape at a time: a token, an OAuth grant, headers or
 * an App each replaces the others when it is written. A form offering all of
 * them at once could only refuse the second one filled in; asking first makes
 * the question the form answers the same one the server stores.
 */
export type AuthChoice = "token" | "headers" | "customHeader" | "oauth" | "githubApp";

/** What a stored credential shows of its shape, without its values. */
export type StoredShape = {
  oauth: boolean;
  githubApp: boolean;
  /** The stored header names, when the read says them. */
  headers: string[];
};

/**
 * The shapes this form can write for the server, in the order offered.
 *
 * `customHeader` is a server with no recipe: a header of its own, such as
 * `apikey`. `githubApp` is offered where the recipe names one, or where one is
 * already stored, so it can be read and taken back.
 */
export function authChoices(
  plan: RemoteAuthPlan,
  extra: { customHeader: boolean; githubApp: boolean },
): AuthChoice[] {
  const out: AuthChoice[] = [];
  if (plan.secret !== null) out.push("token");
  if (headerNames(plan.multiHeaders).length > 0) out.push("headers");
  if (extra.customHeader) out.push("customHeader");
  if (plan.oauth !== null) out.push("oauth");
  if (plan.githubApp !== null || extra.githubApp) out.push("githubApp");
  return out;
}

/**
 * The shape the form opens on: the one stored, when it is one of the choices,
 * else the first offered. Opening on another would invite replacing a working
 * credential by accident, with the first thing typed.
 */
export function initialChoice(
  choices: AuthChoice[],
  stored: StoredShape,
  plan: RemoteAuthPlan,
): AuthChoice | null {
  const offered = (choice: AuthChoice) => choices.includes(choice);
  if (stored.githubApp && offered("githubApp")) return "githubApp";
  if (stored.oauth && offered("oauth")) return "oauth";
  if (stored.headers.length > 0) {
    const recipeHeaders = headerNames(plan.multiHeaders);
    if (offered("headers") && stored.headers.every((name) => recipeHeaders.includes(name))) {
      return "headers";
    }
    if (offered("customHeader") && !stored.headers.some(isAuthorization)) return "customHeader";
  }
  return choices[0] ?? null;
}

function isAuthorization(name: string) {
  return name.trim().toLowerCase() === "authorization";
}
