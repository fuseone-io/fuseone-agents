import type { useForm } from "react-hook-form";
import { MemoryInputField } from "@/features/memory/memory-form-fields";
import type { MemoryFormValues } from "@/features/memory/memory-form-schema";

/**
 * Which run this was learned from.
 *
 * One field, because one is all a person knows. The artifact and the digest
 * used to be typed here: the first almost always says the same thing, and the
 * second is sixty-four characters copied out of one screen to be compared
 * against the record it came from. Both are the ledger's to answer.
 */
export function EvidenceFields({
  control,
}: {
  control: ReturnType<typeof useForm<MemoryFormValues>>["control"];
}) {
  return (
    <MemoryInputField
      control={control}
      name="evidenceRunId"
      label="memory.evidenceRun"
      description="memory.evidenceHint"
      className="font-mono"
    />
  );
}
