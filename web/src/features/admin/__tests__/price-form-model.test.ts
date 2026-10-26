import { describe, expect, it } from "vitest";
import {
  initialPriceFormValues,
  modelPriceFromForm,
} from "@/features/admin/price-form-model";
import type { ModelPrice } from "@/features/admin/prices-api";

describe("price form model", () => {
  it("round-trips the hour-window write rate", () => {
    // The fifth rate is the easy one to drop in a mapping: it is optional,
    // and every other field survives without it.
    const stored: ModelPrice = {
      provider: "anthropic",
      model: "anthropic-claude-opus-5",
      inputMicros: 5_000_000,
      outputMicros: 25_000_000,
      cacheReadMicros: 500_000,
      cacheWriteMicros: 6_250_000,
      cacheWrite1hMicros: 10_000_000,
    };

    const values = initialPriceFormValues(stored);
    expect(values.cacheWrite1hMicros).toBe("10");

    const back = modelPriceFromForm(values);
    expect(back.cacheWrite1hMicros).toBe(10_000_000);
  });

  it("an empty hour rate stays zero, not invented", () => {
    const values = initialPriceFormValues(null);
    expect(modelPriceFromForm({ ...values, provider: "p", model: "m" }).cacheWrite1hMicros).toBe(0);
  });
});
