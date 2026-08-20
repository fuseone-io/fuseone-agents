import { z } from "zod";
import type { ModelPrice } from "@/features/admin/prices-api";

const ratePattern = /^\d+([,.]\d+)?$/;
const rateField = z.string().refine((v) => {
  const trimmed = v.trim();
  return trimmed === "" || ratePattern.test(trimmed);
});

export const priceFormSchema = z.object({
  provider: z.string().trim().min(1),
  model: z.string().trim().min(1),
  inputMicros: rateField,
  outputMicros: rateField,
  cacheReadMicros: rateField,
  cacheWriteMicros: rateField,
});

export type PriceFormValues = z.infer<typeof priceFormSchema>;
export type PriceRateName = keyof Pick<
  PriceFormValues,
  "inputMicros" | "outputMicros" | "cacheReadMicros" | "cacheWriteMicros"
>;

export const PRICE_RATES: { field: PriceRateName; label: string }[] = [
  { field: "inputMicros", label: "admin.rateInput" },
  { field: "outputMicros", label: "admin.rateOutput" },
  { field: "cacheReadMicros", label: "admin.rateCacheRead" },
  { field: "cacheWriteMicros", label: "admin.rateCacheWrite" },
];

export function initialPriceFormValues(
  price: ModelPrice | null,
): PriceFormValues {
  if (!price) {
    return blankPrice("", "");
  }
  if (price.source === "market_default") {
    return blankPrice(price.provider, price.model);
  }
  return {
    provider: price.provider,
    model: price.model,
    inputMicros: rateText(price.inputMicros),
    outputMicros: rateText(price.outputMicros),
    cacheReadMicros: rateText(price.cacheReadMicros),
    cacheWriteMicros: rateText(price.cacheWriteMicros),
  };
}

export function modelPriceFromForm(values: PriceFormValues): ModelPrice {
  return {
    provider: values.provider.trim(),
    model: values.model.trim(),
    inputMicros: rateMicros(values.inputMicros),
    outputMicros: rateMicros(values.outputMicros),
    cacheReadMicros: rateMicros(values.cacheReadMicros),
    cacheWriteMicros: rateMicros(values.cacheWriteMicros),
  };
}

function blankPrice(provider: string, model: string): PriceFormValues {
  return {
    provider,
    model,
    inputMicros: "",
    outputMicros: "",
    cacheReadMicros: "",
    cacheWriteMicros: "",
  };
}

function rateText(micros: number | undefined): string {
  return micros ? String(micros / 1_000_000) : "";
}

function rateMicros(value: string): number {
  const trimmed = value.trim();
  if (trimmed === "") return 0;
  return Math.round(Number(trimmed.replace(",", ".")) * 1_000_000);
}
