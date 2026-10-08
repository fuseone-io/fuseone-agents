import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { StandingForm } from "@/features/admin/standing-form";
import { setLocale } from "@/i18n";

const lists = vi.hoisted(() => ({
  tools: {
    data: {
      items: [{ toolId: "cloudflare.edge.unblock_ip", effect: "write" }],
    },
  },
  agents: {
    data: { items: [{ agentId: "security-sentinel", name: "Sentinel" }] },
  },
  scopes: { data: { items: [{ company: "acme", area: "platform" }] } },
}));

vi.mock("@/features/admin/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/features/admin/api")>()),
  useTools: () => lists.tools,
}));
vi.mock("@/features/agents/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/features/agents/api")>()),
  useAgents: () => lists.agents,
}));
vi.mock("@/features/scope/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/features/scope/api")>()),
  useScopes: () => lists.scopes,
}));

beforeEach(() => {
  setLocale("en-US");
  Element.prototype.hasPointerCapture ??= () => false;
  Element.prototype.scrollIntoView ??= () => {};
});

function mount() {
  render(
    <QueryClientProvider client={new QueryClient()}>
      <StandingForm onClose={vi.fn()} />
    </QueryClientProvider>,
  );
}

describe("the standing approval form", () => {
  // A mandate demands an exact id, which is exactly where typing from
  // memory fails: the governed listings are offered for both fields.
  it("offers the governed tools and the agents", async () => {
    mount();
    await userEvent.click(
      screen.getByRole("combobox", { name: "Suggest governed tools" }),
    );
    await userEvent.click(
      await screen.findByText("cloudflare.edge.unblock_ip"),
    );
    expect(screen.getByLabelText("Tool")).toHaveValue(
      "cloudflare.edge.unblock_ip",
    );

    await userEvent.click(
      screen.getByRole("combobox", { name: "Suggest agents" }),
    );
    await userEvent.click(await screen.findByText("security-sentinel"));
    expect(screen.getByLabelText("Agent")).toHaveValue("security-sentinel");
  });

  // Typing stays valid: a grant may name a tool an operator is about to
  // configure.
  it("still accepts a typed id", async () => {
    mount();
    await userEvent.type(screen.getByLabelText("Tool"), "x");
    expect(screen.getByLabelText("Tool")).toHaveValue("x");
  });
});

// The scope pair suggests the declared scopes, like every list here.
it("offers the declared companies and areas", async () => {
  mount();
  await userEvent.click(
    screen.getByRole("combobox", { name: "Suggest declared areas" }),
  );
  await userEvent.click(await screen.findByText("platform"));
  expect(screen.getByLabelText("Area")).toHaveValue("platform");
});
