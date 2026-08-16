import { describe, expect, it } from "vitest";
import { render, screen } from "@testing-library/react";
import { EffectBadge } from "@/features/admin/effect-badge";

describe("the effect pill", () => {
  /*
   * Two refusals that are not the same job: nobody ruled on this, versus
   * somebody ruled and the tool changed underneath. Both are blocked; only one
   * of them is a decision already made.
   */
  it("says a ruling was overtaken rather than repeating the effect it no longer stands for", () => {
    render(<EffectBadge effect="read" stale />);
    expect(screen.getByText(/revisão/i)).toBeInTheDocument();
    expect(screen.queryByText(/^leitura$/i)).not.toBeInTheDocument();
  });

  it("names the effect in words, so the pill survives a monochrome print", () => {
    render(<EffectBadge effect="destructive" />);
    expect(screen.getByText(/\p{L}/u)).toBeInTheDocument();
  });
});
