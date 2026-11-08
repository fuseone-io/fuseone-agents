import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { PolicyEditorPage } from "@/features/policies/policy-editor-page";
import { PoliciesPage } from "@/features/policies/policies-page";
import { Toaster } from "@/components/ui/sonner";
import { setLocale } from "@/i18n";
import { stubPolicies } from "@/features/policies/__tests__/policy-network";

function at(path: string) {
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <MemoryRouter initialEntries={[path]}>
        <Routes>
          <Route path="/policies" element={<PoliciesPage />} />
          <Route path="/policies/:code" element={<PolicyEditorPage />} />
        </Routes>
      </MemoryRouter>
      <Toaster />
    </QueryClientProvider>,
  );
}

const writes = (net: ReturnType<typeof stubPolicies>, method: string) =>
  net.requests.filter((r) => r.method === method);

describe("a policy written over what was read", () => {
  beforeEach(() => setLocale("en-US"));
  afterEach(() => vi.unstubAllGlobals());

  it("reads the rule by its code and saves under that revision", async () => {
    const net = stubPolicies();
    at("/policies/POL-200");
    await userEvent
      .setup()
      .click(await screen.findByRole("button", { name: "Save and enforce" }));

    await waitFor(() => expect(writes(net, "PUT")).toHaveLength(1));
    expect(
      net.requests.some(
        (r) => r.method === "GET" && r.path === "/policies/POL-200",
      ),
    ).toBe(true);
    expect(writes(net, "PUT")[0]?.ifMatch).toBe('"v1-gen1"');
  });

  it("keeps the edits when somebody wrote first, and reloads only when asked", async () => {
    const user = userEvent.setup();
    const net = stubPolicies();
    at("/policies/POL-200");
    const name = await screen.findByLabelText("Name");
    await user.clear(name);
    await user.type(name, "Mine");
    net.elsewhere({ name: "Theirs" });
    await user.click(screen.getByRole("button", { name: "Save and enforce" }));

    expect(
      await screen.findByText("Somebody changed this while you were editing"),
    ).toBeVisible();
    expect(screen.getByLabelText("Name")).toHaveValue("Mine");
    await user.click(
      screen.getByRole("button", { name: "Reload what is stored" }),
    );
    await waitFor(() =>
      expect(screen.getByLabelText("Name")).toHaveValue("Theirs"),
    );
  });

  it("removes under the revision read when the confirmation opened", async () => {
    const user = userEvent.setup();
    const net = stubPolicies();
    at("/policies");
    await user.click(
      await screen.findByRole("button", { name: /Remove Guard exports/ }),
    );
    net.elsewhere({ name: "Guard exports" });
    await user.click(screen.getByRole("button", { name: "Cancel" }));
    await user.click(
      screen.getByRole("button", { name: /Remove Guard exports/ }),
    );
    await user.click(await screen.findByRole("button", { name: "Remove" }));

    await waitFor(() => expect(writes(net, "DELETE")).toHaveLength(1));
    expect(writes(net, "DELETE")[0]?.ifMatch).toBe('"v1-gen2"');
  });

  it("removes nothing when somebody wrote after the confirmation opened", async () => {
    const user = userEvent.setup();
    const net = stubPolicies();
    at("/policies");
    await user.click(
      await screen.findByRole("button", { name: /Remove Guard exports/ }),
    );
    await screen.findByRole("button", { name: "Remove" });
    net.elsewhere({ name: "Guard exports" });
    await user.click(screen.getByRole("button", { name: "Remove" }));

    expect(await screen.findByText(/nothing was removed/)).toBeVisible();
    expect(net.state.policies).toHaveLength(1);
  });
});
