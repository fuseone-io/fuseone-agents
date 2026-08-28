import { render, screen } from "@testing-library/react";
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
});

function renderEditor(code: string) {
  return render(editor(code));
}

function editor(code: string) {
  return (
    <MemoryRouter initialEntries={[`/policies/${code}`]}>
      <Routes>
        <Route path="/policies/:code" element={<PolicyEditorPage />} />
      </Routes>
    </MemoryRouter>
  );
}

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
