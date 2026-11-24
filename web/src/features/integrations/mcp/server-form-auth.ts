import type { UseFormReturn } from "react-hook-form";
import type { ServerFormValues } from "@/features/integrations/server-schema";
import type { MCPServer } from "@/features/integrations/api";
import type { ServerRecipe } from "@/features/integrations/mcp/api";
import {
  headerCredential,
  headerNames,
  multiHeaderCredential,
  remoteAuthPlan,
  type RemoteAuthPlan,
} from "@/features/integrations/mcp/auth-plan";
import {
  authChoices,
  effectiveChoice,
  type AuthChoice,
  type StoredShape,
} from "@/features/integrations/mcp/auth-choice";
import { customHeaderCredential, customHeaderState } from "@/features/integrations/mcp/custom-header";
import { oauthFromValue, oauthHasValue } from "@/features/integrations/mcp/oauth-credential";
import {
  blankGitHubApp,
  githubAppFromValue,
  githubAppHasValue,
  githubAppHeaders,
} from "@/features/integrations/mcp/github-app-credential";

/** What the connect form may ask of a remote server, and which shape it shows. */
export type ServerFormAuth = {
  plan: RemoteAuthPlan;
  headers: string[];
  choices: AuthChoice[];
  stored: StoredShape;
};

export function serverFormAuth(
  recipe: ServerRecipe | null | undefined,
  server: MCPServer | null | undefined,
): ServerFormAuth {
  const plan = remoteAuthPlan(recipe?.authModes, recipe !== undefined && recipe !== null);
  return {
    plan,
    headers: headerNames(plan.multiHeaders),
    // A custom header only where the recipe was read and there is none:
    // undefined is not knowing yet.
    choices: authChoices(plan, { customHeader: recipe === null, githubApp: Boolean(server?.githubApp) }),
    stored: {
      oauth: server?.hasOAuth === true,
      githubApp: Boolean(server?.githubApp),
      headers: server?.headerNames ?? [],
    },
  };
}

export function formChoice(auth: ServerFormAuth, picked: AuthChoice | null) {
  return effectiveChoice(picked, auth.choices, auth.stored, auth.plan);
}

/**
 * What a save of the connect form sends of the credential: the shape shown,
 * and nothing typed under another. Left empty, the shape sends nothing, which
 * keeps what is stored; removing one is a gesture on the server's own page.
 */
export function formCredential(values: ServerFormValues, auth: ServerFormAuth) {
  if (values.transport !== "http") return {};
  const { plan, headers } = auth;
  switch (formChoice(auth, values.authChoice)) {
    case "token":
      if (values.token === "") return {};
      if (plan.secret?.type === "bearer") return { token: values.token };
      return plan.secret ? { headers: headerCredential(plan.secret, values.token) } : {};
    case "headers":
      return headers.every((header) => (values.headers[header] ?? "").trim() !== "")
        ? { headers: multiHeaderCredential(headers, values.headers) }
        : {};
    case "customHeader": {
      const custom = { name: values.customHeaderName, value: values.customHeaderValue };
      return customHeaderState(custom) === "complete" ? { headers: customHeaderCredential(custom) } : {};
    }
    case "oauth":
      return oauthHasValue(values) ? { oauth: oauthFromValue(values) } : {};
    case "githubApp":
      // The only headers beside an App are its configuration.
      return githubAppHasValue(values)
        ? { githubApp: githubAppFromValue(values), headers: githubAppHeaders(values) }
        : {};
    default:
      return {};
  }
}

// Every typed part of every remote shape, all of them strings.
const SHAPE_TEXT = [
  "token",
  "customHeaderName",
  "customHeaderValue",
  "oauthAccessToken",
  "oauthRefreshToken",
  "oauthTokenURL",
  "oauthClientID",
  "oauthClientSecret",
  "oauthTokenType",
  "oauthExpiresAtUnix",
  "oauthScopes",
  ...(Object.keys(blankGitHubApp()) as (keyof ReturnType<typeof blankGitHubApp>)[]),
] as const;

/** Switching shape starts the new one empty: nothing typed waits under the old one. */
export function clearShapes(form: UseFormReturn<ServerFormValues>) {
  for (const field of SHAPE_TEXT) form.setValue(field, "", { shouldDirty: true });
  form.setValue("headers", {}, { shouldDirty: true });
  form.clearErrors();
}
