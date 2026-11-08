import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { RevisedRemoveButton } from "@/components/shared/revised-remove-button";
import { setLocale } from "@/i18n";

function renderButton(read: () => Promise<string>, onConfirm = vi.fn()) {
  render(
    <QueryClientProvider client={new QueryClient()}>
      <RevisedRemoveButton
        title="Remove github"
        description="Its tools stop."
        revision={{ queryKey: ["test", "github"], read }}
        onConfirm={onConfirm}
      />
    </QueryClientProvider>,
  );
  return onConfirm;
}

describe("removing what was read", () => {
  beforeEach(() => setLocale("en-US"));

  // Read when the confirmation opens, not when the page loaded: the revision a
  // removal answers is the one the person is looking at as they decide.
  it("reads a fresh revision on every opening and removes under it", async () => {
    const user = userEvent.setup();
    const read = vi
      .fn()
      .mockResolvedValueOnce('"v1-one"')
      .mockResolvedValueOnce('"v1-two"');
    const onConfirm = renderButton(read);

    await user.click(screen.getByRole("button", { name: "Remove github" }));
    await user.click(await screen.findByRole("button", { name: "Cancel" }));
    await user.click(screen.getByRole("button", { name: "Remove github" }));
    await user.click(await screen.findByRole("button", { name: "Remove" }));

    expect(read).toHaveBeenCalledTimes(2);
    expect(onConfirm).toHaveBeenCalledWith('"v1-two"');
  });

  it("does not offer to remove what it could not read", async () => {
    const user = userEvent.setup();
    const onConfirm = renderButton(() => Promise.reject(new Error("down")));

    await user.click(screen.getByRole("button", { name: "Remove github" }));

    expect(
      await screen.findByRole("button", { name: "Try again" }),
    ).toBeVisible();
    expect(screen.getByRole("button", { name: "Remove" })).toBeDisabled();
    expect(onConfirm).not.toHaveBeenCalled();
  });

  // The previous revision is still cached while the new one is read; confirming
  // under it would remove over whatever happened in between.
  it("cannot be confirmed while the revision is read again", async () => {
    const user = userEvent.setup();
    let release: (etag: string) => void = () => {};
    const read = vi
      .fn()
      .mockResolvedValueOnce('"v1-one"')
      .mockImplementationOnce(
        () => new Promise<string>((done) => (release = done)),
      );
    renderButton(read);

    await user.click(screen.getByRole("button", { name: "Remove github" }));
    await user.click(await screen.findByRole("button", { name: "Cancel" }));
    await user.click(screen.getByRole("button", { name: "Remove github" }));

    expect(screen.getByRole("button", { name: "Remove" })).toBeDisabled();
    release('"v1-two"');
  });
});
