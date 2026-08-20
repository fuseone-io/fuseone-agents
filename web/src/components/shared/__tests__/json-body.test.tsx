import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { JsonBody } from "@/components/shared/json-body";

describe("a payload in the trail", () => {
  it("colours the parts of an object by what they are", () => {
    const { container } = render(<JsonBody body='{"tool":"crm.note","count":3}' />);

    expect(screen.getByText('"crm.note"')).toHaveClass("text-success");
    expect(screen.getByText("3")).toHaveClass("text-primary");
    // The key is not the value, and reading a payload is mostly finding keys.
    expect(container.querySelectorAll(".text-muted-foreground").length).toBeGreaterThan(0);
  });

  it("shows text that is not JSON instead of failing", () => {
    // A tool returns whatever it returns. An error in prose, XML, a stack
    // trace — rendering blank would be the console losing evidence.
    render(<JsonBody body="upstream said no" />);

    expect(screen.getByText(/upstream said no/)).toBeInTheDocument();
  });

  it("stops colouring a payload too large to walk", () => {
    // One span per node is fine for a tool result and not fine for a dump.
    // Above the cap it is still readable, just not coloured.
    const big = JSON.stringify({
      rows: Array.from({ length: 5000 }, (_, i) => `row-${i}-${"x".repeat(20)}`),
    });
    expect(big.length).toBeGreaterThan(64 * 1024);
    const { container } = render(<JsonBody body={big} />);

    expect(container.querySelector("pre")?.textContent).toContain("row-4999");
    expect(container.querySelectorAll("span").length).toBe(0);
  });

  it("never interprets what a tool returned", () => {
    // The payload is untrusted by definition. React escapes text nodes, and
    // this asserts nobody reached for innerHTML to make colouring faster.
    const { container } = render(
      <JsonBody body='{"note":"<img src=x onerror=alert(1)>"}' />,
    );

    expect(container.querySelector("img")).toBeNull();
    expect(screen.getByText(/onerror/)).toBeInTheDocument();
  });
});
