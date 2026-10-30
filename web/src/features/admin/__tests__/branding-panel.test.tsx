import { fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { BrandingPanel } from "@/features/admin/branding-panel";

const hooks = vi.hoisted(() => ({
  mutate: vi.fn(),
  upload: vi.fn(),
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
  useUploadBrandAsset: () => ({
    mutate: hooks.upload,
    isPending: false,
  }),
}));

vi.mock("sonner", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}));

describe("branding panel", () => {
  beforeEach(() => {
    hooks.mutate.mockReset();
    hooks.upload.mockReset();
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

  it("keeps the colour picker and the hex field on the same value", async () => {
    render(<BrandingPanel />);
    const hex = screen.getByLabelText("Cor primária");
    const picker = screen.getByLabelText("Seletor da cor primária");

    await userEvent.type(hex, "#2357C6");
    expect(picker).toHaveValue("#2357c6");

    // A native colour dialog has no userEvent equivalent; the input event is
    // what the browser fires when somebody picks a colour in it.
    fireEvent.input(picker, { target: { value: "#ff8800" } });
    expect(hex).toHaveValue("#ff8800");
  });

  it("saves the colour chosen in the picker", async () => {
    render(<BrandingPanel />);

    fireEvent.input(screen.getByLabelText("Seletor da cor primária"), {
      target: { value: "#ff8800" },
    });
    await userEvent.click(screen.getByRole("button", { name: "Salvar" }));

    expect(hooks.mutate).toHaveBeenCalledWith(
      expect.objectContaining({ primaryColor: "#ff8800" }),
      expect.anything(),
    );
  });

  it("puts the uploaded logo's internal address in the logo field", async () => {
    const url = `/branding/assets/${"a".repeat(64)}`;
    hooks.upload.mockImplementation((_file, options) =>
      options.onSuccess({ sha256: "a".repeat(64), url, contentType: "image/png" }),
    );
    render(<BrandingPanel />);

    const png = new File([new Uint8Array(64)], "logo.png", { type: "image/png" });
    await userEvent.upload(screen.getByLabelText("Enviar logo"), png);

    expect(hooks.upload).toHaveBeenCalledWith(png, expect.anything());
    expect(screen.getByLabelText("URL do logo")).toHaveValue(url);
  });

  it("refuses an image over 512 KiB before sending it", async () => {
    render(<BrandingPanel />);

    const big = new File([new Uint8Array(512 * 1024 + 1)], "logo.png", {
      type: "image/png",
    });
    await userEvent.upload(screen.getByLabelText("Enviar ícone"), big);

    expect(hooks.upload).not.toHaveBeenCalled();
    expect(screen.getByText(/passa do limite/)).toBeInTheDocument();
  });
});
