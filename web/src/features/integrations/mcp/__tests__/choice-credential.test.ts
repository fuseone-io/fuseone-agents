import { describe, expect, it } from "vitest";
import { remoteAuthPlan } from "@/features/integrations/mcp/auth-plan";
import { authChoices, initialChoice } from "@/features/integrations/mcp/auth-choice";
import { chosenCredential } from "@/features/integrations/mcp/choice-credential";
import { blankCredential } from "@/features/integrations/mcp/credential-value";

/*
A save sends the shape chosen and nothing of the others, whatever the form's
state still holds. Switching also clears that state on screen; this holds on
its own, so neither guard hides a missing other.
*/

const plan = remoteAuthPlan(
  [
    { type: "bearer", principal: "service" },
    { type: "headers", principal: "service", headers: ["Api-Key"] },
    { type: "oauth2", principal: "user" },
  ],
  true,
);

const everything = {
  ...blankCredential(),
  token: "bearer",
  headers: { "Api-Key": "key" },
  oauthAccessToken: "access",
};

describe("the credential a save sends", () => {
  it.each([
    ["token", { token: "bearer" }],
    ["headers", { headers: { "Api-Key": "key" } }],
    ["oauth", { oauth: expect.objectContaining({ accessToken: "access" }) }],
  ] as const)("for %s, is that shape alone", (choice, want) => {
    expect(chosenCredential(choice, everything, plan, ["Api-Key"])).toEqual(want);
  });

  it("is nothing when no shape is chosen", () => {
    expect(chosenCredential(null, everything, plan, ["Api-Key"])).toEqual({});
  });
});

describe("the shape a form opens on", () => {
  const choices = authChoices(plan, { customHeader: false, githubApp: false });

  it.each([
    ["an OAuth grant", { oauth: true, githubApp: false, headers: [] }, "oauth"],
    ["the recipe's headers", { oauth: false, githubApp: false, headers: ["Api-Key"] }, "headers"],
    ["a token", { oauth: false, githubApp: false, headers: [] }, "token"],
  ] as const)("is the one stored, for %s", (_what, stored, want) => {
    expect(initialChoice(choices, { ...stored, headers: [...stored.headers] }, plan)).toBe(want);
  });

  it("is a custom header for a server with no recipe holding one", () => {
    const unknown = remoteAuthPlan(undefined, false);
    const offered = authChoices(unknown, { customHeader: true, githubApp: false });
    expect(initialChoice(offered, { oauth: false, githubApp: false, headers: ["apikey"] }, unknown))
      .toBe("customHeader");
  });
});
