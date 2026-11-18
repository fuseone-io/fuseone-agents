import type { AgentDefinition } from "@/lib/api/client";

/** An empty definition, for tests that build one field at a time. */
export const BLANK: AgentDefinition = {
  name: "",
  company: "",
  area: "",
  provider: "",
  model: "",
  instructions: "",
  tools: [],
  budget: { micros: 500_000, steps: 60 },
  triggers: [],
};
