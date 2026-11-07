import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  aToken,
  anAccount,
  SECRET,
  stubAccounts,
} from "@/features/admin/__tests__/service-accounts-network";
import { ServiceAccountsPanel } from "@/features/admin/service-accounts-panel";
import { setLocale } from "@/i18n";

beforeEach(() => {
  setLocale("en-US");
  Element.prototype.hasPointerCapture ??= () => false;
  Element.prototype.scrollIntoView ??= () => {};
});
afterEach(() => vi.unstubAllGlobals());

function mount() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <ServiceAccountsPanel />
    </QueryClientProvider>,
  );
  return client;
}

async function openPublisher(user: ReturnType<typeof userEvent.setup>) {
  await user.click(await screen.findByRole("button", { name: "Manage publisher" }));
  await screen.findByRole("heading", { name: "Tokens" });
}

async function issue(user: ReturnType<typeof userEvent.setup>, button: string) {
  await user.click(await screen.findByRole("button", { name: button }));
  await user.click(screen.getByRole("combobox", { name: "Valid for" }));
  await user.click(await screen.findByRole("option", { name: "90 days" }));
  await user.click(screen.getByRole("button", { name: "Issue this token" }));
  return screen.findByRole("alertdialog", { name: "Copy this token now" });
}

/** Everything TanStack keeps, as it would be seen by anything reading the cache. */
function cached(client: QueryClient) {
  return JSON.stringify([
    client.getQueryCache().getAll().map((q) => q.state.data),
    client.getMutationCache().getAll().map((m) => [m.state.data, m.state.variables]),
  ]);
}

describe("the service accounts list", () => {
  it("says what would appear here when there are none", async () => {
    stubAccounts({ accounts: [] });
    mount();
    expect(await screen.findByText("No service accounts yet")).toBeInTheDocument();
  });

  it("offers a retry when the list cannot be read", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response(null, { status: 500 })));
    mount();
    expect(await screen.findByRole("button", { name: /try again/i })).toBeInTheDocument();
  });

  it("creates an account through the route, holding nothing", async () => {
    const user = userEvent.setup();
    const { requests } = stubAccounts();
    mount();
    await user.click(await screen.findByRole("button", { name: "New service account" }));
    await user.type(screen.getByLabelText("Name"), "terraform");
    await user.type(screen.getByLabelText("What it is for"), "applies the plan");
    await user.click(screen.getByRole("button", { name: "Create account" }));

    await screen.findByRole("button", { name: "Manage terraform" });
    expect(requests).toEqual([
      {
        method: "POST",
        path: "/admin/service-accounts",
        body: { name: "terraform", description: "applies the plan" },
      },
    ]);
  });

  it("refuses a name the server would refuse, before asking it", async () => {
    const user = userEvent.setup();
    const { requests } = stubAccounts();
    mount();
    await user.click(await screen.findByRole("button", { name: "New service account" }));
    await user.type(screen.getByLabelText("Name"), "Not Valid");
    await user.click(screen.getByRole("button", { name: "Create account" }));

    expect(await screen.findByText(/lowercase letters, digits and hyphens/i)).toBeInTheDocument();
    expect(requests).toEqual([]);
  });
});

describe("issuing a token", () => {
  it("will not issue one without a validity", async () => {
    const user = userEvent.setup();
    const { requests } = stubAccounts();
    mount();
    await openPublisher(user);
    await user.click(screen.getByRole("button", { name: "Issue token" }));
    await user.type(screen.getByLabelText("Token name"), "deploy");
    await user.click(screen.getByRole("button", { name: "Issue this token" }));

    expect(await screen.findByText(/choose how long/i)).toBeInTheDocument();
    expect(requests).toEqual([]);
  });

  it("sends the validity chosen as an instant in the future", async () => {
    const user = userEvent.setup();
    const { requests } = stubAccounts();
    mount();
    await openPublisher(user);
    await user.click(screen.getByRole("button", { name: "Issue token" }));
    await user.type(screen.getByLabelText("Token name"), "deploy");
    await user.click(screen.getByRole("combobox", { name: "Valid for" }));
    await user.click(await screen.findByRole("option", { name: "90 days" }));
    await user.click(screen.getByRole("button", { name: "Issue this token" }));
    await screen.findByRole("alertdialog");

    const body = requests[0]?.body as { name: string; expiresAt: string };
    const days = (Date.parse(body.expiresAt) - Date.now()) / 86_400_000;
    expect(body.name).toBe("deploy");
    expect(Math.round(days)).toBe(90);
  });

  it("shows the secret once, and it is gone after the dialog closes", async () => {
    const user = userEvent.setup();
    stubAccounts();
    const client = mount();
    await openPublisher(user);
    await issue(user, "Rotate ci");
    expect(screen.getByText(SECRET)).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "I have saved it" }));

    await waitFor(() => expect(screen.queryByRole("alertdialog")).toBeNull());
    expect(screen.queryByText(SECRET)).toBeNull();
    expect(cached(client)).not.toContain(SECRET);
  });

  // The reader's copy of the secret is the only one anywhere. A cache that kept
  // it would hand it to whatever reads the cache, for as long as the tab lives.
  it("keeps the secret out of every cache while it is on screen", async () => {
    const user = userEvent.setup();
    stubAccounts();
    const client = mount();
    await openPublisher(user);
    await issue(user, "Rotate ci");

    await waitFor(() => expect(client.isMutating()).toBe(0));
    expect(screen.getByText(SECRET)).toBeInTheDocument();
    expect(cached(client)).not.toContain(SECRET);
  });

  it("cannot be dismissed by accident, because nothing shows it again", async () => {
    const user = userEvent.setup();
    stubAccounts();
    mount();
    await openPublisher(user);
    await issue(user, "Rotate ci");

    await user.keyboard("{Escape}");

    expect(screen.getByText(SECRET)).toBeInTheDocument();
  });

  it("copies the secret", async () => {
    const user = userEvent.setup();
    stubAccounts();
    mount();
    await openPublisher(user);
    const dialog = await issue(user, "Rotate ci");

    await user.click(within(dialog).getByRole("button", { name: "Copy" }));

    expect(await navigator.clipboard.readText()).toBe(SECRET);
  });

  it("does not offer a token on a disabled account", async () => {
    const user = userEvent.setup();
    stubAccounts({ accounts: [anAccount({ disabled: true })] });
    mount();
    await openPublisher(user);

    expect(screen.queryByRole("button", { name: "Issue token" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Rotate ci" })).toBeNull();
    expect(screen.getByText(/enable the account to issue/i)).toBeInTheDocument();
  });
});

describe("rotating a token", () => {
  it("reuses the name, leaves the old one working, and says what to do next", async () => {
    const user = userEvent.setup();
    const { requests } = stubAccounts();
    mount();
    await openPublisher(user);
    const dialog = await issue(user, "Rotate ci");

    expect(requests).toHaveLength(1);
    expect(requests[0]).toMatchObject({ method: "POST", body: { name: "ci" } });
    expect(within(dialog).getByText(/then revoke tok_old/i)).toBeInTheDocument();
  });

  it("tells two tokens with one name apart", async () => {
    const user = userEvent.setup();
    stubAccounts();
    mount();
    await openPublisher(user);
    await issue(user, "Rotate ci");
    await user.click(screen.getByRole("button", { name: "I have saved it" }));

    expect(await screen.findByText("tok_new")).toBeInTheDocument();
    expect(screen.getByText("tok_old")).toBeInTheDocument();
  });
});

describe("revoking a token", () => {
  it("asks first, and asking is not revoking", async () => {
    const user = userEvent.setup();
    const { requests } = stubAccounts();
    mount();
    await openPublisher(user);
    await user.click(await screen.findByRole("button", { name: "Revoke ci" }));
    expect(await screen.findByRole("alertdialog")).toHaveTextContent("tok_old");

    await user.click(screen.getByRole("button", { name: "Cancel" }));

    expect(requests).toEqual([]);
  });

  it("calls the route and the row says so, with nothing left to do to it", async () => {
    const user = userEvent.setup();
    const { requests } = stubAccounts();
    mount();
    await openPublisher(user);
    await user.click(await screen.findByRole("button", { name: "Revoke ci" }));
    await user.click(await screen.findByRole("button", { name: "Revoke token" }));

    expect(await screen.findByText("revoked")).toBeInTheDocument();
    expect(requests).toEqual([
      { method: "DELETE", path: "/admin/service-accounts/svc_publisher/tokens/tok_old" },
    ]);
    expect(screen.queryByRole("button", { name: "Revoke ci" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Rotate ci" })).toBeNull();
  });

  it("shows an expired token as expired, which needs a replacement and not a revocation", async () => {
    const user = userEvent.setup();
    stubAccounts({ tokens: [aToken({ expiresAt: "2020-01-01T00:00:00Z" })] });
    mount();
    await openPublisher(user);

    expect(await screen.findByText("expired")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Revoke ci" })).toBeNull();
    expect(screen.getByRole("button", { name: "Rotate ci" })).toBeInTheDocument();
  });
});

describe("disabling an account", () => {
  it("asks first, then calls its own route and touches no token", async () => {
    const user = userEvent.setup();
    const { requests } = stubAccounts();
    mount();
    await openPublisher(user);
    await user.click(screen.getByRole("button", { name: "Disable account" }));
    await user.click(
      within(await screen.findByRole("alertdialog")).getByRole("button", { name: "Disable" }),
    );

    await screen.findByRole("button", { name: "Enable account" });
    expect(requests).toEqual([
      {
        method: "PUT",
        path: "/admin/service-accounts/svc_publisher/disabled",
        body: { disabled: true },
      },
    ]);
  });

  // Enabling is the one act here that gives access back, all at once. It reads
  // as harmless, so it says how much it returns before it does it.
  it("says how many tokens enabling brings back, not counting dead ones", async () => {
    const user = userEvent.setup();
    stubAccounts({
      accounts: [anAccount({ disabled: true })],
      tokens: [
        aToken({ id: "tok_a" }),
        aToken({ id: "tok_b", revokedAt: "2026-11-02T00:00:00Z" }),
        aToken({ id: "tok_c", expiresAt: "2020-01-01T00:00:00Z" }),
      ],
    });
    mount();
    await openPublisher(user);
    await screen.findByText("tok_a");
    await user.click(screen.getByRole("button", { name: "Enable account" }));

    expect(await screen.findByRole("alertdialog")).toHaveTextContent(/1 token/);
  });
});

describe("enabling needs to know what it gives back", () => {
  const disabled = () => ({ accounts: [anAccount({ disabled: true })] });
  const enableButton = () => screen.getByRole("button", { name: "Enable" });

  async function askToEnable(user: ReturnType<typeof userEvent.setup>) {
    await user.click(screen.getByRole("button", { name: "Enable account" }));
    return screen.findByRole("alertdialog");
  }

  it("will not enable while the tokens are still being read", async () => {
    const user = userEvent.setup();
    const { requests, state } = stubAccounts(disabled());
    state.tokensRead = () => new Promise(() => {});
    mount();
    await openPublisher(user);
    const dialog = await askToEnable(user);

    expect(dialog).not.toHaveTextContent(/\d+ tokens?/);
    expect(enableButton()).toBeDisabled();
    await user.click(enableButton());
    expect(requests).toEqual([]);
  });

  it("will not enable when the tokens could not be read, and offers a retry", async () => {
    const user = userEvent.setup();
    const { requests, state } = stubAccounts(disabled());
    state.tokensRead = async () => new Response(null, { status: 500 });
    mount();
    await openPublisher(user);
    const dialog = await askToEnable(user);

    const retry = await within(dialog).findByRole("button", { name: /try again/i });
    expect(dialog).not.toHaveTextContent(/\d+ tokens?/);
    expect(enableButton()).toBeDisabled();

    state.tokensRead = undefined;
    await user.click(retry);
    await within(dialog).findByText(/1 token/);
    expect(requests).toEqual([]);
    expect(enableButton()).toBeEnabled();
  });

  // The list on screen can predate somebody else's token. The number is read
  // when it is asked for, and a failure then is a failure, whatever was cached.
  it("counts what is there now, not what the screen last read", async () => {
    const user = userEvent.setup();
    const { state } = stubAccounts(disabled());
    mount();
    await openPublisher(user);
    await screen.findByText("tok_old");
    state.tokens = [...state.tokens, aToken({ id: "tok_other" })];
    const dialog = await askToEnable(user);

    await within(dialog).findByText(/2 tokens/);
  });

  it("will not enable on an earlier count when reading it again fails", async () => {
    const user = userEvent.setup();
    const { requests, state } = stubAccounts(disabled());
    mount();
    await openPublisher(user);
    await screen.findByText("tok_old");
    state.tokensRead = async () => new Response(null, { status: 500 });
    const dialog = await askToEnable(user);

    await within(dialog).findByRole("button", { name: /try again/i });
    expect(dialog).not.toHaveTextContent(/\d+ tokens?/);
    expect(enableButton()).toBeDisabled();
    expect(requests).toEqual([]);
  });

  it("does not take a read it abandoned as the answer to a newer question", async () => {
    const user = userEvent.setup();
    const { state } = stubAccounts(disabled());
    mount();
    await openPublisher(user);
    await screen.findByText("tok_old");
    state.tokensRead = () => new Promise(() => {});
    await askToEnable(user);
    await user.click(screen.getByRole("button", { name: "Cancel" }));
    await askToEnable(user);

    // Long enough for the abandoned read to settle.
    await new Promise((resolve) => setTimeout(resolve, 20));
    expect(enableButton()).toBeDisabled();
  });

  it("does not make disabling wait for the list: it is how to stop a machine", async () => {
    const user = userEvent.setup();
    const { requests, state } = stubAccounts();
    state.tokensRead = async () => new Response(null, { status: 500 });
    mount();
    await openPublisher(user);
    await user.click(screen.getByRole("button", { name: "Disable account" }));
    await user.click(await screen.findByRole("button", { name: "Disable" }));

    await waitFor(() => expect(requests).toHaveLength(1));
    expect(requests[0]).toMatchObject({ method: "PUT", body: { disabled: true } });
  });
});

describe("what an account may do", () => {
  async function changing(user: ReturnType<typeof userEvent.setup>) {
    await openPublisher(user);
    await user.click(screen.getByRole("button", { name: "Change access" }));
  }

  it("says a machine never approves or manages identity, whatever its role", async () => {
    const user = userEvent.setup();
    stubAccounts();
    mount();
    await changing(user);

    expect(screen.getByText(/can never approve an action/i)).toBeInTheDocument();
    // A machine has no sign-in, so the person's sentence about one would be false.
    expect(screen.queryByText(/next sign-in/i)).toBeNull();
  });

  it("does not offer the approver role or the administrator shortcut", async () => {
    const user = userEvent.setup();
    stubAccounts();
    mount();
    await changing(user);
    await user.click(screen.getByRole("combobox", { name: "Role" }));

    expect(await screen.findByRole("option", { name: "Author" })).toBeInTheDocument();
    expect(screen.queryByRole("option", { name: "Approver" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Grant administrator" })).toBeNull();
  });

  it("still shows an approver role it already holds, so it can be taken away", async () => {
    const user = userEvent.setup();
    stubAccounts({
      accounts: [
        anAccount({ grants: [{ company: "acme", area: "cx", role: "approver" }] }),
      ],
    });
    mount();
    await changing(user);

    expect(screen.getByRole("combobox", { name: "Role" })).toHaveTextContent("Approver");
  });

  it("saves through the grants route and leaves tokens alone", async () => {
    const user = userEvent.setup();
    const { requests } = stubAccounts();
    mount();
    await changing(user);
    await user.click(screen.getByRole("button", { name: "Remove this access" }));
    await user.click(screen.getByRole("button", { name: "Save" }));

    await screen.findByText("holds nothing yet");
    expect(requests).toEqual([
      { method: "PUT", path: "/admin/people/svc_publisher/grants", body: { grants: [] } },
    ]);
  });
});
