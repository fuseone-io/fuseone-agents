import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeAll, describe, expect, it, vi } from "vitest";
import { ModelField } from "@/features/agents/model-field";

/*
 * The accuser for the truncated button (#168 review). The trigger clamps a
 * long model id so it cannot overflow its grid track, which is only honest
 * if the full id stays readable somewhere: a custom model is not in the
 * provider's suggestions, so without being folded into the list it would be
 * truncated on the button and shown whole nowhere on the form.
 */
describe("the model combobox", () => {
  beforeAll(() => {
    Element.prototype.hasPointerCapture ??= () => false;
    Element.prototype.setPointerCapture ??= () => {};
    Element.prototype.releasePointerCapture ??= () => {};
    Element.prototype.scrollIntoView ??= () => {};
  });

  it("shows a custom model absent from the suggestions in full when opened", async () => {
    const user = userEvent.setup();
    const custom =
      "litellm/anthropic/claude-sonnet-5-custom-deployment-us-central1";
    render(
      <ModelField
        id="agent-model"
        value={custom}
        options={["gpt-5", "devstack"]}
        onChange={vi.fn()}
      />,
    );

    await user.click(screen.getByRole("combobox"));

    expect(await screen.findByRole("option", { name: custom })).toBeTruthy();
  });
});
