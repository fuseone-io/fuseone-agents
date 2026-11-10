import { z } from "zod";
import type { ExperimentRequest } from "@/features/experiments/experiments-api";

/** The sentinel an empty override is held as, since a Select item cannot be "". */
export const AS_PUBLISHED = "published";

export const experimentForm = z.object({
  version: z.string().min(1),
  provider: z.string(),
  model: z.string(),
  effort: z.string(),
  repetitions: z.enum(["1", "2", "3", "4", "5"]),
});

export type ExperimentForm = z.infer<typeof experimentForm>;

export function experimentDefaults(version: string): ExperimentForm {
  return { version, provider: AS_PUBLISHED, model: "", effort: AS_PUBLISHED, repetitions: "1" };
}

/**
 * The request a form asks for.
 *
 * A field left as published is left out, never sent empty, and a form that
 * overrides nothing sends no configuration at all: that is the version as
 * published, the one configuration that keeps its declared fallback. Any
 * override, even of effort alone, runs without it.
 */
export function experimentRequestOf(values: ExperimentForm): ExperimentRequest {
  const config = {
    provider: values.provider === AS_PUBLISHED ? undefined : values.provider,
    model: values.model === "" ? undefined : values.model,
    effort: values.effort === AS_PUBLISHED ? undefined : values.effort,
  };
  const overrides = Object.values(config).some((v) => v !== undefined);
  return {
    version: values.version,
    repetitions: Number(values.repetitions),
    config: overrides ? config : undefined,
  };
}
