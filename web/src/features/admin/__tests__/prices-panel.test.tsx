import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { PricesPanel } from "@/features/admin/prices-panel";
import type { ModelPrice } from "@/features/admin/prices-api";
import { setInstallationCurrency } from "@/lib/format";

const hooks = vi.hoisted(() => ({
  prices: [] as ModelPrice[],
  money: { currency: "EUR" },
  refetch: vi.fn(),
  remove: vi.fn(),
  put: vi.fn(),
  setMoney: vi.fn(),
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

vi.mock("@/features/money/api", () => ({
  useMoney: () => ({ data: hooks.money }),
  useSetMoney: () => ({ mutate: hooks.setMoney, isPending: false }),
}));

vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

describe("price administration", () => {
  beforeEach(() => {
    hooks.prices = [];
    setInstallationCurrency("EUR");
    hooks.refetch.mockReset();
    hooks.refetch.mockResolvedValue({});
    hooks.remove.mockReset();
    hooks.put.mockReset();
    hooks.setMoney.mockReset();
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
    expect(screen.getByText(/Não converte os números já gravados/)).toBeInTheDocument();
    expect(screen.getByText(/US\$/)).toBeInTheDocument();
    expect(screen.getByText(/€/)).toBeInTheDocument();
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

  it("saves the installation currency as a normalized code", async () => {
    const user = userEvent.setup();

    render(<PricesPanel />);

    const currency = screen.getByLabelText("Moeda da instalação");
    await user.clear(currency);
    await user.type(currency, "usd");
    await user.click(screen.getByRole("button", { name: "Salvar" }));

    await waitFor(() =>
      expect(hooks.setMoney).toHaveBeenCalledWith(
        { currency: "USD" },
        expect.any(Object),
      ),
    );
  });

  it("keeps decimal rates editable until they are saved as micros", async () => {
    const user = userEvent.setup();
    hooks.prices = [
      {
        provider: "anthropic",
        model: "claude-haiku-4-5",
        source: "market_default",
        currency: "USD",
        inputMicros: 500_000,
        outputMicros: 2_500_000,
      },
    ];

    render(<PricesPanel />);

    await user.click(screen.getByText("anthropic/claude-haiku-4-5"));
    const input = screen.getByLabelText("Entrada / milhão");
    await user.type(input, "0.5");

    expect(input).toHaveValue("0.5");

    const save = screen.getAllByRole("button", { name: "Salvar" }).at(-1);
    expect(save).toBeDefined();
    await user.click(save!);

    await waitFor(() =>
      expect(hooks.put).toHaveBeenCalledWith(
        expect.objectContaining({
          provider: "anthropic",
          model: "claude-haiku-4-5",
          inputMicros: 500_000,
        }),
        expect.any(Object),
      ),
    );
  });

  it("offers known providers and models without copying reference prices", async () => {
    const user = userEvent.setup();
    hooks.prices = [
      {
        provider: "anthropic",
        model: "claude-haiku-4-5",
        source: "market_default",
        currency: "USD",
        inputMicros: 500_000,
        outputMicros: 2_500_000,
      },
      {
        provider: "openai",
        model: "gpt-4o-mini",
        source: "market_default",
        currency: "USD",
        inputMicros: 150_000,
        outputMicros: 600_000,
      },
    ];

    render(<PricesPanel />);

    await user.click(screen.getByRole("button", { name: "Novo" }));
    await user.click(screen.getByRole("button", { name: "anthropic" }));

    expect(screen.getByLabelText("Provedor")).toHaveValue("anthropic");
    expect(
      screen.getByRole("button", { name: "claude-haiku-4-5" }),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "gpt-4o-mini" }),
    ).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "claude-haiku-4-5" }));

    expect(screen.getByLabelText("Modelo")).toHaveValue("claude-haiku-4-5");
    expect(screen.getByLabelText("Entrada / milhão")).toHaveValue("");

    const save = screen.getAllByRole("button", { name: "Salvar" }).at(-1);
    expect(save).toBeDefined();
    await user.click(save!);

    await waitFor(() =>
      expect(hooks.put).toHaveBeenCalledWith(
        expect.objectContaining({
          provider: "anthropic",
          model: "claude-haiku-4-5",
          inputMicros: 0,
          outputMicros: 0,
        }),
        expect.any(Object),
      ),
    );
  });
});
