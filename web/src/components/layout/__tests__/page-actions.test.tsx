import { describe, expect, it } from "vitest";
import { render, screen } from "@testing-library/react";
import { Bot } from "lucide-react";
import {
  PageActionsProvider,
  PageActionsTarget,
} from "@/components/layout/page-actions";
import { PageHeader } from "@/components/shared/page-header";

describe("a screen's primary action", () => {
  it("renders in the header rather than in the page", () => {
    render(
      <PageActionsProvider>
        <header data-testid="chrome">
          <PageActionsTarget />
        </header>
        <main data-testid="page">
          <PageHeader icon={Bot} title="Agentes">
            <button>Novo agente</button>
          </PageHeader>
        </main>
      </PageActionsProvider>,
    );

    const action = screen.getByRole("button", { name: "Novo agente" });
    expect(screen.getByTestId("chrome")).toContainElement(action);
    expect(screen.getByTestId("page")).not.toContainElement(action);
  });

  it("keeps the action visible when there is no header to put it in", () => {
    // A PageHeader rendered without the shell — in a test, or before anybody
    // has signed in — must not drop its action somewhere nobody can see.
    render(
      <PageHeader icon={Bot} title="Agentes">
        <button>Novo agente</button>
      </PageHeader>,
    );

    expect(
      screen.getByRole("button", { name: "Novo agente" }),
    ).toBeInTheDocument();
  });

  it("leaves with the page that owns it", () => {
    // The reason this is a portal and not shared state: unmounting the page
    // takes the button with it, so the header never shows the last screen's
    // action while the next one loads.
    const { rerender } = render(
      <PageActionsProvider>
        <header data-testid="chrome">
          <PageActionsTarget />
        </header>
        <PageHeader icon={Bot} title="Agentes">
          <button>Novo agente</button>
        </PageHeader>
      </PageActionsProvider>,
    );
    expect(
      screen.getByRole("button", { name: "Novo agente" }),
    ).toBeInTheDocument();

    rerender(
      <PageActionsProvider>
        <header data-testid="chrome">
          <PageActionsTarget />
        </header>
        <PageHeader icon={Bot} title="Custo" />
      </PageActionsProvider>,
    );
    expect(
      screen.queryByRole("button", { name: "Novo agente" }),
    ).not.toBeInTheDocument();
  });
});
