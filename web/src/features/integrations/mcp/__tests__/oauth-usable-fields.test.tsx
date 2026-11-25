import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it } from "vitest";
import { blankCredential } from "@/features/integrations/mcp/credential-value";
import { OAuthFields } from "@/features/integrations/mcp/oauth-fields";
import { setLocale } from "@/i18n";

// A disabled save says why, or the person is left guessing what is missing.
describe("a person's manual OAuth grant with no token", () => {
  beforeEach(() => setLocale("en-US"));

  function show(typed: Partial<ReturnType<typeof blankCredential>>) {
    render(
      <OAuthFields
        value={{ ...blankCredential(), ...typed }}
        stored={{ hasOAuth: false }}
        invalidExpiry={false}
        onChange={() => {}}
        onRevoke={() => {}}
      />,
    );
  }

  it("says it has nothing to send, and where the client belongs", () => {
    show({ oauthClientID: "agents", oauthTokenURL: "https://idp.example.com/token" });
    expect(screen.getByText(/has nothing to send/)).toHaveTextContent("Browser sign-in");
  });

  it("says nothing once a token is given", () => {
    show({ oauthClientID: "agents", oauthAccessToken: "access" });
    expect(screen.queryByText(/has nothing to send/)).not.toBeInTheDocument();
  });
});
