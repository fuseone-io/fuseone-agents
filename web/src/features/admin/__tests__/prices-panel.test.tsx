import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { PricesPanel } from "@/features/admin/prices-panel";
import type { ModelPrice } from "@/features/admin/prices-api";

const hooks = vi.hoisted(() => ({
  prices: [] as ModelPrice[],
  refetch: vi.fn(),
  remove: vi.fn(),
  put: vi.fn(),
}));

vi.mock("@/features/admin/prices-api", async (importOriginal) => {
  const actual =
    await importOriginal<typeof import("@/features/admin/prices-api")>();
  return {
    ...actual,
    usePrices: () => ({
      data: { items: hooks.prices },
      isLoading: false,
      error: null,
      refetch: hooks.refetch,
    }),
    useDeletePrice: () => ({ mutate: hooks.remove, isPending: false }),
    usePutPrice: () => ({ mutate: hooks.put, isPending: false }),
  };
});

vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

describe("price administration", () => {
  beforeEach(() => {
    hooks.refetch.mockReset();
    hooks.refetch.mockResolvedValue({});
    hooks.remove.mockReset();
    hooks.put.mockReset();
  });

  it("separates bundled market defaults from configured overrides", async () => {
    hooks.prices = [
      {
        provider: "anthropic",
        model: "claude-opus-5",
        source: "market_default",
        currency: "USD",
        inputMicros: 2_500_000,
        outputMicros: 12_500_000,
        sourceUpdatedAt: "2026-08-20",
      },
      {
        provider: "anthropic",
        model: "claude-sonnet-5",
        source: "configured",
        inputMicros: 2_500_000,
        outputMicros: 11_000_000,
      },
    ];

    render(<PricesPanel />);

    expect(screen.getByText("default de mercado")).toBeInTheDocument();
    expect(screen.getByText("tarifa própria")).toBeInTheDocument();
    expect(screen.getByText(/US\$/)).toBeInTheDocument();
    expect(screen.getByText(/Apenas referência em USD/)).toBeInTheDocument();
    expect(
      screen.queryByRole("button", {
        name: "Remover a tarifa de claude-opus-5?",
      }),
    ).not.toBeInTheDocument();
    expect(
      screen.getByRole("button", {
        name: "Remover a tarifa de claude-sonnet-5?",
      }),
    ).toBeInTheDocument();

    await userEvent.click(screen.getByText("anthropic/claude-opus-5"));
    expect(screen.getByText("Sobrescrever o default de mercado")).toBeInTheDocument();
    expect(screen.getByLabelText("Entrada / milhão")).toHaveValue("");
  });
});
