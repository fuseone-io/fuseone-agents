import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {
  MemoryRouter,
  Route,
  Routes,
  useLocation,
} from "react-router-dom";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { PoliciesTable } from "@/features/policies/policies-table";
import { setLocale } from "@/i18n";
import type { Policy } from "@/lib/api/client";

describe("policy list actions", () => {
  beforeEach(() => setLocale("en-US"));

  it("opens the selected policy from the edit button", async () => {
    const user = userEvent.setup();
    renderTable();

    await user.click(screen.getByRole("link", { name: "Edit Guard exports" }));

    expect(screen.getByTestId("location")).toHaveTextContent(
      "/policies/POL-200",
    );
  });

  it("opens the selected policy from the row outside its name", async () => {
    const user = userEvent.setup();
    renderTable();
    const row = screen.getByText("Grace").closest("tr");

    expect(
      screen.getByRole("link", { name: /^Guard exports/ }),
    ).toHaveAttribute("href", "/policies/POL-200");
    expect(row).not.toBeNull();
    await user.click(within(row!).getByText("Grace"));

    expect(screen.getByTestId("location")).toHaveTextContent(
      "/policies/POL-200",
    );
  });

  it("names and removes only the policy that was confirmed", async () => {
    const user = userEvent.setup();
    const onDelete = vi.fn();
    renderTable(onDelete);

    await user.click(
      screen.getByRole("button", { name: "Remove Guard exports" }),
    );

    expect(onDelete).not.toHaveBeenCalled();
    expect(screen.getByRole("alertdialog")).toHaveTextContent("POL-200");
    expect(screen.getByRole("alertdialog")).toHaveTextContent("Guard exports");

    await user.click(screen.getByRole("button", { name: "Remove" }));

    expect(onDelete).toHaveBeenCalledOnce();
    expect(onDelete).toHaveBeenCalledWith("POL-200");
    expect(screen.getByTestId("location")).toHaveTextContent("/policies");
  });

  it("does not offer write actions without policy management permission", () => {
    renderTable(vi.fn(), false);

    expect(screen.queryByRole("link", { name: /Edit / })).toBeNull();
    expect(screen.queryByRole("button", { name: /Remove / })).toBeNull();
    expect(screen.queryByRole("columnheader", { name: "Actions" })).toBeNull();
  });
});

function renderTable(onDelete = vi.fn(), canManage = true) {
  render(
    <MemoryRouter initialEntries={["/policies"]}>
      <Routes>
        <Route
          path="*"
          element={
            <>
              <PoliciesTable
                policies={policies}
                canManage={canManage}
                onDelete={onDelete}
              />
              <Location />
            </>
          }
        />
      </Routes>
    </MemoryRouter>,
  );
}

function Location() {
  return <output data-testid="location">{useLocation().pathname}</output>;
}

const policies: Policy[] = [
  policy({ code: "POL-100", name: "Review refunds", owner: "Ada" }),
  policy({ code: "POL-200", name: "Guard exports", owner: "Grace" }),
];

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
