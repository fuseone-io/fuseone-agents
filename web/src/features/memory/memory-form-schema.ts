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
  evidenceArtifact: z.string().min(1),
  evidenceDigest: z.string().min(1),
  reason: z.string().min(1),
});

export type MemoryFormValues = z.infer<typeof memoryFormSchema>;
