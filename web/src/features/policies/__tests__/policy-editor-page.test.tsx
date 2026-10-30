import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { PolicyEditorPage } from "@/features/policies/policy-editor-page";
import { setLocale } from "@/i18n";
import type { Policy } from "@/lib/api/client";

const hooks = vi.hoisted(() => ({
  policies: {
    data: undefined as { items: Policy[] } | undefined,
    isLoading: true,
    error: null as Error | null,
    refetch: vi.fn(),
  },
  put: vi.fn(),
  simulate: vi.fn(),
}));

vi.mock("@/features/policies/api", () => ({
  usePolicies: () => hooks.policies,
  usePutPolicy: () => ({ mutate: hooks.put, isPending: false }),
  useSimulatePolicy: () => ({
    mutate: hooks.simulate,
    isPending: false,
    data: undefined,
  }),
}));

describe("the policy editor", () => {
  beforeEach(() => {
    setLocale("en-US");
    hooks.policies.data = undefined;
    hooks.policies.isLoading = true;
    hooks.policies.error = null;
    hooks.put.mockClear();
  });

  it("seeds the edit draft after the first policy request finishes", () => {
    const view = renderEditor("POL-200");

    hooks.policies.data = {
      items: [
        policy({
          code: "POL-200",
          name: "Guard exports",
          owner: "Grace",
          reason: "Keep customer data inside the installation",
        }),
      ],
    };
    hooks.policies.isLoading = false;
    view.rerender(editor("POL-200"));

    expect(screen.getByLabelText("Name")).toHaveValue("Guard exports");
    expect(screen.getByLabelText("Owner")).toHaveValue("Grace");
    expect(screen.getByLabelText("Reason")).toHaveValue(
      "Keep customer data inside the installation",
    );
  });

  // The draft is the form's state; what is sent is what was chosen on screen.
  it("saves what was edited, under the record's code", async () => {
    const user = userEvent.setup();
    loadPolicy(policy({ code: "POL-200", name: "Guard", mode: "monitor" }));
    renderEditor("POL-200");

    await user.click(screen.getByRole("radio", { name: /Allow/ }));
    await user.click(screen.getByRole("radio", { name: /Enforce/ }));
    await user.click(screen.getByRole("button", { name: "Save and enforce" }));

    await waitFor(() =>
      expect(hooks.put).toHaveBeenCalledWith(
        {
          code: "POL-200",
          policy: expect.objectContaining({
            name: "Guard",
            effect: "allow",
            mode: "enforce",
          }),
        },
        expect.anything(),
      ),
    );
  });

  it("cannot be saved while the reach names no agent", async () => {
    const user = userEvent.setup();
    loadPolicy(policy({ code: "POL-200", name: "Guard" }));
    renderEditor("POL-200");
    const save = screen.getByRole("button", { name: "Save and enforce" });
    expect(save).toBeEnabled();

    await user.click(screen.getByRole("radio", { name: /^Agents/ }));

    expect(save).toBeDisabled();
  });

  it("cannot be saved without a name", async () => {
    const user = userEvent.setup();
    loadPolicy(policy({ code: "POL-200", name: "Guard" }));
    renderEditor("POL-200");
    const save = screen.getByRole("button", { name: "Save and enforce" });
    expect(save).toBeEnabled();

    await user.clear(screen.getByLabelText("Name"));

    expect(save).toBeDisabled();
  });
});

function loadPolicy(loaded: Policy) {
  hooks.policies.data = { items: [loaded] };
  hooks.policies.isLoading = false;
}

function renderEditor(code: string) {
  return render(editor(code));
}

function editor(code: string) {
  return (
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[`/policies/${code}`]}>
        <Routes>
          <Route path="/policies/:code" element={<PolicyEditorPage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>
  );
}

const client = new QueryClient();

function policy(over: Partial<Policy>): Policy {
  return {
    code: "POL-100",
    name: "Rule",
    resource: "*",
    effects: [],
    conditions: [],
    effect: "deny",
    mode: "enforce",
    enabled: true,
    ...over,
  };
}
