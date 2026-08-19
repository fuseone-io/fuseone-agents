import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeAll, beforeEach, describe, expect, it } from "vitest";
import { ChannelForm } from "@/features/channels/channel-form";
import { setLocale } from "@/i18n";

function renderForm() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });

  return render(
    <QueryClientProvider client={client}>
      <ChannelForm channel={null} kinds={["slack"]} onClose={() => {}} />
    </QueryClientProvider>,
  );
}

describe("channel connection form", () => {
  beforeAll(() => {
    Element.prototype.hasPointerCapture ??= () => false;
    Element.prototype.setPointerCapture ??= () => {};
    Element.prototype.releasePointerCapture ??= () => {};
    Element.prototype.scrollIntoView ??= () => {};
  });

  beforeEach(() => setLocale("en-US"));

  it("asks for the Socket Mode app token only when socket delivery is selected", async () => {
    const user = userEvent.setup();
    renderForm();

    expect(screen.getByLabelText("Signing secret")).toBeInTheDocument();
    expect(
      screen.queryByLabelText("Socket Mode app token"),
    ).not.toBeInTheDocument();

    await user.click(screen.getByRole("combobox", { name: /How asks arrive/ }));
    await user.click(await screen.findByRole("option", { name: "Socket Mode" }));

    expect(screen.getByLabelText("Socket Mode app token")).toBeInTheDocument();
    expect(screen.queryByLabelText("Signing secret")).not.toBeInTheDocument();
  });
});
