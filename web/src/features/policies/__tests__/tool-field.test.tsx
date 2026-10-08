import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ToolField } from "@/features/policies/tool-field";
import { setLocale } from "@/i18n";

const lists = vi.hoisted(() => ({
  tools: {
    data: {
      items: [
        { toolId: "cloudflare.edge.block_ip", effect: "write" },
        { toolId: "grafana.query_loki_logs", effect: "read" },
      ],
    },
  },
}));

vi.mock("@/features/admin/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/features/admin/api")>()),
  useTools: () => lists.tools,
}));

beforeEach(() => {
  setLocale("en-US");
  Element.prototype.hasPointerCapture ??= () => false;
  Element.prototype.scrollIntoView ??= () => {};
});

describe("the policy tool field", () => {
  // The suggestions are the governed ids; picking fills the field.
  it("offers the governed tools", async () => {
    const onChange = vi.fn();
    render(<ToolField value="" onChange={onChange} />);
    await userEvent.click(
      screen.getByRole("combobox", { name: "Suggest governed tools" }),
    );
    await userEvent.click(await screen.findByText("cloudflare.edge.block_ip"));
    expect(onChange).toHaveBeenCalledWith("cloudflare.edge.block_ip");
  });

  // A glob is a legitimate rule, so typing stays just as valid: one
  // keystroke must arrive in onChange, or the field stopped accepting
  // free text.
  it("still accepts free text and globs", async () => {
    const onChange = vi.fn();
    render(<ToolField value="" onChange={onChange} />);
    await userEvent.type(screen.getByLabelText("Tool"), "*");
    expect(onChange).toHaveBeenCalledWith("*");
  });
});
