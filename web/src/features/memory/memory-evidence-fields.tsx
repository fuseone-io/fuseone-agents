import type { useForm } from "react-hook-form";
import { MemoryInputField } from "@/features/memory/memory-form-fields";
import type { MemoryFormValues } from "@/features/memory/memory-form-schema";

export function EvidenceFields({
  control,
}: {
  control: ReturnType<typeof useForm<MemoryFormValues>>["control"];
}) {
  return (
    <div className="grid gap-3 rounded-md border p-3">
      <MemoryInputField
        control={control}
        name="evidenceRunId"
        label="memory.evidenceRun"
        className="font-mono"
      />
      <div className="grid gap-3 sm:grid-cols-2">
        <MemoryInputField
          control={control}
          name="evidenceArtifact"
          label="memory.evidenceArtifact"
          className="font-mono"
        />
        <MemoryInputField
          control={control}
          name="evidenceDigest"
          label="memory.evidenceDigest"
          className="font-mono"
        />
      </div>
    </div>
  );
}
