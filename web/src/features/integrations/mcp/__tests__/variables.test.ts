import { describe, expect, it } from "vitest";
import { readVariables, writeVariables } from "@/features/integrations/mcp/variables";

describe("reading the variables a local server is given", () => {
  it("keeps a value containing an equals sign whole", () => {
    // Tokens end in padding. Split on every `=` and the credential arrives
    // cut in half, failing somewhere nobody connects back to this box.
    expect(readVariables("TOKEN=abc==")).toEqual({ TOKEN: "abc==" });
  });

  it("drops a line that names nothing rather than guessing at it", () => {
    expect(readVariables("GITHUB_TOKEN=x\njust-a-note\n# comment")).toEqual({
      GITHUB_TOKEN: "x",
    });
  });

  it("round-trips, so an edit does not rewrite what was not touched", () => {
    const env = { REGION: "eu-west-1", TOKEN: "abc==" };
    expect(readVariables(writeVariables(env))).toEqual(env);
  });
});
