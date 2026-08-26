import type { MemoryAssertionInput } from "@/features/memory/api";
import type { MemoryFormValues } from "@/features/memory/memory-form-schema";

export function memoryDefaults(company: string, area: string): MemoryFormValues {
  return {
    company: company === "*" ? "" : company,
    area,
    agentId: "",
    kind: "",
    subject: "",
    signature: "",
    claim: "",
    observations: "1",
    confirmed: "1",
    evidenceRunId: "",
    evidenceArtifact: "final_answer",
    evidenceDigest: "",
    reason: "",
  };
}

export function toMemoryAssertionInput(
  values: MemoryFormValues,
): MemoryAssertionInput {
  return {
    company: values.company.trim(),
    area: values.area.trim(),
    agentId: trimOrUndefined(values.agentId),
    kind: values.kind.trim(),
    subject: values.subject.trim(),
    signature: values.signature.trim(),
    claim: values.claim.trim(),
    observations: Number(values.observations),
    confirmed: Number(values.confirmed),
    evidence: [
      {
        runId: values.evidenceRunId.trim(),
        artifact: values.evidenceArtifact.trim(),
        digest: values.evidenceDigest.trim(),
      },
    ],
    reason: values.reason.trim(),
  };
}

function trimOrUndefined(value: string): string | undefined {
  const trimmed = value.trim();
  return trimmed === "" ? undefined : trimmed;
}
