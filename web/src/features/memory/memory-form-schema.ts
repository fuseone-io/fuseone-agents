import { z } from "zod";

export const memoryFormSchema = z.object({
  company: z.string().min(1),
  area: z.string().min(1),
  namespace: z.enum(["agent", "shared"]),
  kind: z.string().min(1),
  subject: z.string().min(1),
  signature: z.string().min(1),
  claim: z.string().min(1).max(1200),
  evidenceRunId: z.string().min(1),
  /**
   * Which of the run's outputs, when the screen knows. Empty leaves it to the
   * server, which answers the closing answer — the citation the console has
   * always made and the one a person naming only a run means.
   *
   * Never typed. On the memory page it stays empty; in a run's sheet it holds a
   * name read out of the finished step, chosen from what the ledger recorded.
   */
  evidenceArtifact: z.string(),
  reason: z.string().min(1),
});

export type MemoryFormValues = z.infer<typeof memoryFormSchema>;
