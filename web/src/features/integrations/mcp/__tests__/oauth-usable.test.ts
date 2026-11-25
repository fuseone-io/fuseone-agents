import { describe, expect, it } from "vitest";
import { chosenReady } from "@/features/integrations/mcp/choice-credential";
import { blankCredential } from "@/features/integrations/mcp/credential-value";
import { serverSchema, type ServerFormValues } from "@/features/integrations/server-schema";

/*
A manual grant with no token is refused before the round trip, as the server
refuses it: the client typed in alone is the browser sign-in's configuration
in the wrong place, and saved it replaced the credential that worked.
*/

const clientOnly = {
  oauthClientID: "agents",
  oauthTokenURL: "https://idp.example.com/token",
  oauthScopes: "openid",
};

const remote = {
  name: "dpg", transport: "http", scopeKind: "installation", company: "", area: "",
  protocolMode: "auto", command: "", args: "", url: "https://mcp.example.com/mcp",
  authChoice: "oauth", token: "", headers: {}, customHeaderName: "", customHeaderValue: "",
  env: "", dsn: "", oauthAccessToken: "", oauthRefreshToken: "", oauthTokenURL: "",
  oauthClientID: "", oauthClientSecret: "", oauthTokenType: "", oauthExpiresAtUnix: "",
  oauthScopes: "", githubAppId: "", githubInstallationId: "", githubPrivateKey: "",
  githubRepositories: "", githubAPIBaseURL: "", githubToolsets: "", configFile: "",
  configFileEnv: "", callTimeoutSeconds: "", rateLimitPerSecond: "", rateLimitBurst: "",
  cacheTTLSeconds: "", cacheMaxEntries: "", stdioEgressMode: "inherit",
  stdioEgressDestinations: "", acceptsLocalExecution: false, enabled: true,
} as ServerFormValues;

function issues(values: ServerFormValues) {
  const parsed = serverSchema.safeParse(values);
  return parsed.success ? [] : parsed.error.issues.map((issue) => issue.message);
}

describe("a manual OAuth grant with no token", () => {
  it("is refused by the server's form, saying where the client belongs", () => {
    expect(issues({ ...remote, ...clientOnly })).toContain("mcp.oauthNothingToSend");
  });

  it.each([
    ["an access token", { oauthAccessToken: "access" }],
    ["a refresh token and its token URL", { oauthRefreshToken: "refresh", oauthTokenURL: clientOnly.oauthTokenURL }],
  ])("is saved with %s", (_, tokens) => {
    expect(issues({ ...remote, ...clientOnly, ...tokens })).toEqual([]);
  });

  it("keeps a person's save disabled", () => {
    expect(chosenReady("oauth", { ...blankCredential(), ...clientOnly }, [])).toBe(false);
  });
});
