import { describe, expect, it } from "vitest";
import { formChoice, formCredential, serverFormAuth } from "@/features/integrations/mcp/server-form-auth";
import type { ServerFormValues } from "@/features/integrations/server-schema";

/*
The connect form sends the shape shown and nothing typed under another,
whatever its state still holds. Switching also clears that state on screen;
this holds on its own, so neither guard hides a missing other.
*/

const auth = serverFormAuth(
  {
    server: "crm", title: "CRM", category: "operations", publisher: "CRM", docsFrom: "publisher",
    provenance: "documentation", status: "published", configRequirements: ["credential"],
    requiresPersonalCredential: false, transport: "http", url: "https://crm.example/mcp",
    authModes: [
      { type: "bearer", principal: "service" },
      { type: "headers", principal: "service", headers: ["Api-Key"] },
      { type: "oauth2", principal: "user" },
      { type: "github_app", principal: "installation" },
    ],
  },
  null,
);

const everything = {
  transport: "http", token: "bearer", headers: { "Api-Key": "key" },
  customHeaderName: "apikey", customHeaderValue: "custom",
  oauthAccessToken: "access", oauthRefreshToken: "", oauthTokenURL: "", oauthClientID: "",
  oauthClientSecret: "", oauthTokenType: "", oauthExpiresAtUnix: "", oauthScopes: "",
  githubAppId: "1", githubInstallationId: "2", githubPrivateKey: "pem",
  githubRepositories: "", githubAPIBaseURL: "", githubToolsets: "",
} as unknown as ServerFormValues;

describe("the credential the connect form sends", () => {
  it.each([
    ["token", ["token"]],
    ["headers", ["headers"]],
    ["oauth", ["oauth"]],
    ["githubApp", ["githubApp", "headers"]],
  ] as const)("for %s, is that shape alone", (choice, keys) => {
    const sent = formCredential({ ...everything, authChoice: choice }, auth);
    expect(Object.keys(sent).sort()).toEqual([...keys].sort());
  });

  // A server with no recipe: the custom header is one of its shapes.
  it("for a custom header, is that header alone", () => {
    const sent = formCredential({ ...everything, authChoice: "customHeader" }, serverFormAuth(null, null));
    expect(sent).toEqual({ headers: { apikey: "custom" } });
  });

  it("is nothing for a local server", () => {
    expect(formCredential({ ...everything, transport: "stdio", authChoice: "token" }, auth)).toEqual({});
  });
});

describe("the shape the connect form shows", () => {
  const oauthOnly = serverFormAuth(
    {
      server: "docs", title: "Docs", category: "knowledge", publisher: "Docs", docsFrom: "publisher",
      provenance: "documentation", status: "published", configRequirements: ["credential"],
      requiresPersonalCredential: false, transport: "http", url: "https://docs.example/mcp",
      authModes: [{ type: "oauth2", principal: "user" }],
    },
    null,
  );

  // Picked before the recipe arrived, a shape it does not offer is a guess.
  it("follows a recipe that arrived after a pick it does not offer", () => {
    expect(formChoice(oauthOnly, "token")).toBe("oauth");
  });

  it("opens on the stored shape before anything is picked", () => {
    const stored = serverFormAuth(null, { name: "crm", enabled: true, hasOAuth: true });
    expect(formChoice(stored, null)).toBe("oauth");
  });
});
