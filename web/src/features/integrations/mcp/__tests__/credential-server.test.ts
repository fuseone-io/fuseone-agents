import { describe, expect, it } from "vitest";
import { credentialServers } from "@/features/integrations/mcp/credential-server";

/*
Whether a server is one people sign in to, and whether every call takes their
own credential, read the same whichever answer the person may read: the
servers' listing for whoever configures them, the credentials' own for anyone
else.
*/
describe("the servers a person keeps a credential for", () => {
  it("are read from the listing, its sign-in and call policy said", () => {
    const [server] = credentialServers([{
      name: "dpg", transport: "http", enabled: true,
      oauthLogin: { clientID: "agents", tokenAuthMethod: "none", hasClientSecret: false },
      callAuth: { policy: "personal_required", callerHasPersonalCredential: false },
    }], undefined);
    expect(server).toMatchObject({ name: "dpg", signIn: true, requiresPersonal: true });
  });

  it("are read from the credentials' own answer for someone who cannot list servers", () => {
    const [server] = credentialServers(undefined, [{
      name: "dpg", scopeKind: "area", shared: false, signIn: true, requiresPersonal: true,
    }]);
    expect(server).toMatchObject({ name: "dpg", transport: "http", signIn: true, requiresPersonal: true });
  });

  it("say neither for a server with no sign-in and a shared credential", () => {
    const [server] = credentialServers([{ name: "plain", transport: "http", enabled: true }], undefined);
    expect(server).toMatchObject({ signIn: false, requiresPersonal: false });
  });
});
