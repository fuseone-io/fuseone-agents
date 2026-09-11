import { describe, expect, it } from "vitest";
import { modelsFor } from "@/features/agents/model-field";

/*
The models offered to an author come from the installation, not from a preset.

A preset ships the names a vendor serves, and behind a proxy those describe a
different endpoint that happens to share a name. The one control that exists so
nobody has to spell `anthropic-claude-sonnet-5` from memory was offering the
vendor's list instead — and the field was empty for every provider the platform
ships no list for.
*/
describe("the models suggested for a provider", () => {
  const presets = [{ name: "anthropic", models: ["claude-opus-5"] }];

  it("are the ones configured for that endpoint", () => {
    const configured = [
      { name: "anthropic", models: ["anthropic-claude-sonnet-5"] },
    ];
    expect(modelsFor("anthropic", configured, presets)).toEqual([
      "anthropic-claude-sonnet-5",
    ]);
  });

  it("fall back to the preset when the installation configured none", () => {
    expect(modelsFor("anthropic", [{ name: "anthropic" }], presets)).toEqual([
      "claude-opus-5",
    ]);
  });

  // A provider the platform ships no list for used to offer nothing at all,
  // which is the case a proxy always is.
  it("are offered for a provider no preset describes", () => {
    const configured = [{ name: "litellm", models: ["gemini/gemini-2.5-pro"] }];
    expect(modelsFor("litellm", configured, presets)).toEqual([
      "gemini/gemini-2.5-pro",
    ]);
  });
});
