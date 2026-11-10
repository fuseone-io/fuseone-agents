import { describe, expect, it } from "vitest";
import { experimentDefaults, experimentRequestOf } from "@/features/experiments/experiment-form";

/*
What a form asks for is the configuration the fallback rule reads (NT-015): nothing
overridden is the version as published and keeps its fallback, and any override —
effort alone included — runs without it. So an untouched form must send no
configuration at all, and a partial one only what it changed.
*/

describe("the request a form asks for", () => {
  it("sends no configuration when nothing was overridden", () => {
    const request = experimentRequestOf(experimentDefaults("v2"));

    expect(request).toEqual({ version: "v2", repetitions: 1, config: undefined });
  });

  it("sends effort alone when only effort changed", () => {
    const request = experimentRequestOf({ ...experimentDefaults("v2"), effort: "low", repetitions: "3" });

    expect(request.config).toEqual({ provider: undefined, model: undefined, effort: "low" });
    expect(request.repetitions).toBe(3);
  });

  it("sends a provider alone, keeping the version's model", () => {
    const request = experimentRequestOf({ ...experimentDefaults("v2"), provider: "openai" });

    expect(request.config).toEqual({ provider: "openai", model: undefined, effort: undefined });
  });
});
