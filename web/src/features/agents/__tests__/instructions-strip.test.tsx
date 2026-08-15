import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { InstructionsStrip } from "@/features/agents/instructions-strip";

/*
The size of an instruction, in whichever unit was actually measured.

The console has no tokeniser and cannot acquire one that stays right, so the
number is the provider's or it is not a token count. Printing characters under
the word "tokens" would be a wrong number in the one place somebody goes to
size a prompt.
*/

const SUMMARY = { tools: 2, stops: 1, characters: 1840 };

describe("what the instructions card adds up to", () => {
  it("shows the count the provider measured", () => {
    render(
      <InstructionsStrip
        summary={SUMMARY}
        findings={[]}
        tokens={412}
      />,
    );

    expect(screen.getByText(/412 tokens/)).toBeInTheDocument();
    expect(screen.queryByText(/caracteres/)).not.toBeInTheDocument();
  });

  it("shows characters, and calls them characters, when nothing counted", () => {
    render(<InstructionsStrip summary={SUMMARY} findings={[]} />);

    expect(screen.getByText(/1840 caracteres/)).toBeInTheDocument();
    expect(screen.queryByText(/tokens/)).not.toBeInTheDocument();
  });
});
