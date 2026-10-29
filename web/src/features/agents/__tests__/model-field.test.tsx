import { render, screen, within } from "@testing-library/react";
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
    // No space, hyphen or other natural break opportunity: the worst case
    // for a flex item under an overflow-hidden list, which would hold its
    // min-content width and be clipped without a forced break.
    const custom = "litellmanthropicclaudesonnet5deployment" + "x".repeat(40);
    render(
      <ModelField
        id="agent-model"
        value={custom}
        options={["gpt-5", "devstack"]}
        onChange={vi.fn()}
      />,
    );

    await user.click(screen.getByRole("combobox"));

    // The option is in the list, and its text is allowed to wrap: jsdom lays
    // nothing out, so the breaking class is the measurable mechanism.
    const option = await screen.findByRole("option", { name: custom });
    const label = within(option).getByText(custom);
    expect(label.className).toContain("break-all");
  });
});
