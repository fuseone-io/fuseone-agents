import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ProviderForm } from "@/features/integrations/provider-form";
import { ScopeChoiceContext } from "@/features/integrations/scope/scope-choice";

const api = vi.hoisted(() => ({ mutateAsync: vi.fn() }));

vi.mock("@/features/integrations/api", async (importOriginal) => {
  const actual =
    await importOriginal<typeof import("@/features/integrations/api")>();
  return {
    ...actual,
    usePutProvider: () => ({ mutateAsync: api.mutateAsync }),
    useIntegrations: () => ({ data: { presets: [] } }),
  };
});

vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

// File scope, so the Radix shims and the mock reset reach every test in
// every describe — scoped to one block, the suite only passed because the
// earlier tests happened to run first and install them.
beforeEach(() => {
  Element.prototype.hasPointerCapture ??= () => false;
  Element.prototype.scrollIntoView ??= () => {};
  api.mutateAsync.mockReset();
  api.mutateAsync.mockResolvedValue(undefined);
});

describe("the model provider sheet", () => {
  // Behind a proxy the model names are the installation's own, so somebody has
  // to be able to say what they are. Pasted one per line, which is how a proxy
  // lists them.
  it("stores the models an endpoint serves, one per line", async () => {
    render(<ProviderForm provider={null} onClose={vi.fn()} />);

    await userEvent.type(screen.getByLabelText("Nome"), "litellm");
    await userEvent.type(
      screen.getByLabelText("Endereço"),
      "https://litellm.internal/v1",
    );
    await userEvent.type(
      screen.getByLabelText("Modelos que este endpoint serve"),
      "anthropic-claude-sonnet-5\n gemini/gemini-2.5-pro \n\n",
    );
    await userEvent.click(screen.getByRole("button", { name: "Salvar" }));

    expect(api.mutateAsync).toHaveBeenCalledWith(
      expect.objectContaining({
        name: "litellm",
        models: ["anthropic-claude-sonnet-5", "gemini/gemini-2.5-pro"],
      }),
    );
  });

  // Editing an address must not empty the list beside it: the field is filled
  // from what is stored, not from what a preset would have said.
  it("keeps the stored models when the sheet opens on a provider", () => {
    render(
      <ProviderForm
        provider={{
          name: "litellm",
          kind: "openai_compatible",
          baseUrl: "https://litellm.internal/v1",
          models: ["anthropic-claude-sonnet-5"],
          enabled: true,
          hasKey: true,
        }}
        onClose={vi.fn()}
      />,
    );

    expect(
      screen.getByLabelText("Modelos que este endpoint serve"),
    ).toHaveValue("anthropic-claude-sonnet-5");
  });
});

describe("the declared tool_choice-auto list", () => {
  const anthropic = {
    name: "litellm",
    kind: "anthropic" as const,
    baseUrl: "https://litellm.internal",
    models: [],
    autoToolChoiceModels: ["anthropic-claude-opus-5-5"],
    enabled: true,
    hasKey: true,
  };

  // The declaration exists because a fallback-routing gateway swallows the
  // refusal the planner would otherwise learn from. It is pasted one per
  // line and cleaned exactly like the model list.
  it("shows on an Anthropic provider and submits the cleaned list", async () => {
    render(
      <ProviderForm
        provider={{ ...anthropic, autoToolChoiceModels: [] }}
        onClose={vi.fn()}
      />,
    );

    await userEvent.type(
      screen.getByLabelText("Modelos conduzidos com tool_choice auto"),
      "anthropic-claude-opus-5-5\n gemini/gemini-2.5-pro \n\n",
    );
    await userEvent.click(screen.getByRole("button", { name: "Salvar" }));

    expect(api.mutateAsync).toHaveBeenCalledWith(
      expect.objectContaining({
        autoToolChoiceModels: [
          "anthropic-claude-opus-5-5",
          "gemini/gemini-2.5-pro",
        ],
      }),
    );
  });

  // Only the Anthropic path sends tool_choice. Switching the protocol must
  // take the promise off the screen AND out of the payload — a stored list
  // no code path reads is configuration that lies.
  it("disappears and submits empty when the protocol is not Anthropic", async () => {
    render(<ProviderForm provider={anthropic} onClose={vi.fn()} />);

    await userEvent.click(screen.getByRole("combobox", { name: "Protocolo" }));
    await userEvent.click(
      screen.getByRole("option", { name: "Compatível com OpenAI" }),
    );

    expect(
      screen.queryByLabelText("Modelos conduzidos com tool_choice auto"),
    ).not.toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Salvar" }));

    expect(api.mutateAsync).toHaveBeenCalledWith(
      expect.objectContaining({ autoToolChoiceModels: [] }),
    );
  });
});

// NT-019 phase 3: a provider lives at a scope. An area's administrator stores
// it in the area they configure; the platform at the installation by default.
describe("where a provider is stored", () => {
  it("stores an area administrator's new provider in their area", async () => {
    render(
      <ScopeChoiceContext.Provider value={{ anyScope: false, configure: [{ company: "acme", area: "cx" }] }}>
        <ProviderForm provider={null} onClose={vi.fn()} />
      </ScopeChoiceContext.Provider>,
    );
    await userEvent.type(screen.getByLabelText("Nome"), "cx-llm");
    await userEvent.type(screen.getByLabelText("Endereço"), "https://cx.example.com/v1");
    await userEvent.click(screen.getByRole("button", { name: "Salvar" }));

    expect(api.mutateAsync).toHaveBeenCalledWith(
      expect.objectContaining({ scope: expect.objectContaining({ scopeKind: "area", company: "acme", area: "cx" }) }),
    );
  });

  it("stores the platform's new provider at the installation by default", async () => {
    render(<ProviderForm provider={null} onClose={vi.fn()} />);
    await userEvent.type(screen.getByLabelText("Nome"), "shared");
    await userEvent.type(screen.getByLabelText("Endereço"), "https://api.example.com/v1");
    await userEvent.click(screen.getByRole("button", { name: "Salvar" }));

    expect(api.mutateAsync).toHaveBeenCalledWith(
      expect.objectContaining({ scope: expect.objectContaining({ scopeKind: "installation" }) }),
    );
  });
});
