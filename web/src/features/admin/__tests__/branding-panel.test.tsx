import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { BrandingPanel } from "@/features/admin/branding-panel";

const hooks = vi.hoisted(() => ({
  mutate: vi.fn(),
  data: {
    displayName: "FuseOne Agents",
    logoUrl: "",
    iconUrl: "",
    primaryColor: "",
  },
}));

vi.mock("@/features/branding/api", () => ({
  useAdminBranding: () => ({
    data: hooks.data,
    isLoading: false,
    error: null,
    refetch: vi.fn(),
  }),
  useSetAdminBranding: () => ({
    mutate: hooks.mutate,
    isPending: false,
  }),
}));

vi.mock("sonner", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}));

describe("branding panel", () => {
  beforeEach(() => {
    hooks.mutate.mockReset();
  });

  it("saves the display name, images and primary colour the operator entered", async () => {
    render(<BrandingPanel />);

    await userEvent.clear(screen.getByLabelText("Nome exibido"));
    await userEvent.type(screen.getByLabelText("Nome exibido"), "Acme Agents");
    await userEvent.type(
      screen.getByLabelText("URL do logo"),
      "https://assets.example.com/logo.svg",
    );
    await userEvent.type(
      screen.getByLabelText("URL do ícone"),
      "https://assets.example.com/icon.png",
    );
    await userEvent.type(screen.getByLabelText("Cor primária"), "#2357C6");

    await userEvent.click(screen.getByRole("button", { name: "Salvar" }));

    expect(hooks.mutate).toHaveBeenCalledWith(
      {
        displayName: "Acme Agents",
        logoUrl: "https://assets.example.com/logo.svg",
        iconUrl: "https://assets.example.com/icon.png",
        primaryColor: "#2357C6",
      },
      expect.objectContaining({ onSuccess: expect.any(Function) }),
    );
  });

  it("does not submit a colour that is not a hex value", async () => {
    render(<BrandingPanel />);

    await userEvent.type(screen.getByLabelText("Cor primária"), "2357C6");

    expect(screen.getByRole("button", { name: "Salvar" })).toBeDisabled();
    expect(hooks.mutate).not.toHaveBeenCalled();
  });

  it("warns that external image URLs leak availability and network access", () => {
    render(<BrandingPanel />);

    expect(
      screen.getByText(/URL externa não carrega em instalação sem saída/i),
    ).toBeInTheDocument();
    expect(
      screen.getByText(/avisa esse endereço sempre que alguém abre a tela/i),
    ).toBeInTheDocument();
  });
});
