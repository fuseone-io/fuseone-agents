import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { PolicyEditorPage } from "@/features/policies/policy-editor-page";
import { setLocale } from "@/i18n";

vi.mock("@/features/policies/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/features/policies/api")>()),
  usePolicies: () => ({
    data: { items: [] },
    isLoading: false,
    error: null,
    refetch: vi.fn(),
  }),
  usePutPolicy: () => ({ mutate: vi.fn(), isPending: false }),
  useSimulatePolicy: () => ({
    mutate: vi.fn(),
    isPending: false,
    data: undefined,
  }),
}));

beforeEach(() => setLocale("en-US"));

// The one-of-many choices used to be buttons carrying role="radio" by hand.
// That markup claims to be a radio and behaves like a button: no group to name
// it, no arrow keys, every option a separate tab stop. These tests hold the
// editor to what the role promises a keyboard or screen-reader user.
describe("the policy editor's one-of-many choices", () => {
  it("are radio groups, each with a name", () => {
    renderNewPolicy();
    for (const name of [
      "Effect when the rule matches",
      "Enforcement",
      "Reach",
    ]) {
      expect(screen.getByRole("radiogroup", { name })).toBeInTheDocument();
    }
  });

  it("move the effect with the arrow keys", async () => {
    const user = userEvent.setup();
    renderNewPolicy();

    await user.click(screen.getByRole("radio", { name: /Deny/ }));
    // Held, as a finger holds it: Radix moves focus a tick after keydown and
    // selects on that focus only while the arrow is still down.
    await user.keyboard("{ArrowLeft>}");
    await waitFor(() =>
      expect(screen.getByRole("radio", { name: /Escalate/ })).toBeChecked(),
    );
    await user.keyboard("{/ArrowLeft}");
    expect(screen.getByRole("radio", { name: /Deny/ })).not.toBeChecked();
  });
});

function renderNewPolicy() {
  render(
    <QueryClientProvider client={new QueryClient()}>
      <MemoryRouter initialEntries={["/policies/new"]}>
        <Routes>
          <Route path="/policies/:code" element={<PolicyEditorPage />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}
