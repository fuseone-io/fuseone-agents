import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ComponentProps } from "react";
import { beforeAll, beforeEach, describe, expect, it } from "vitest";
import { ChannelForm } from "@/features/channels/channel-form";
import { setLocale } from "@/i18n";

function renderForm(
  channel: ComponentProps<typeof ChannelForm>["channel"] = null,
) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });

  return render(
    <QueryClientProvider client={client}>
      <ChannelForm channel={channel} kinds={["slack"]} onClose={() => {}} />
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
    await user.click(
      await screen.findByRole("option", { name: "Socket Mode" }),
    );

    expect(screen.getByLabelText("Socket Mode app token")).toBeInTheDocument();
    expect(screen.queryByLabelText("Signing secret")).not.toBeInTheDocument();
  });
});

/*
 * A connection this console cannot draw is not offered for editing.
 *
 * Its delivery mode came from a newer version, and the platform keeps it: a way
 * of being reached that nothing here can name opens no inbound door. The form
 * would fill the field with this console's idea of the nearest value — "http",
 * the one with a door — and write that back on save.
 */
describe("a delivery mode this console does not know", () => {
  it("is not offered for editing", () => {
    renderForm({
      name: "acme-slack",
      kind: "slack",
      deliveryMode: "a-future-mode",
      enabled: true,
      hasCredential: true,
      conversations: [],
    });

    expect(screen.getByText(/a-future-mode/)).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Save" }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("combobox", { name: /How asks arrive/ }),
    ).not.toBeInTheDocument();
  });
});
