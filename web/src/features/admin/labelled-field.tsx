import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

/** One labelled input. The description travels as an object because seven
 *  separate props is past the point where a call site reads. */
export interface FieldSpec {
  id: string;
  label: string;
  value: string;
  type?: string;
  hint?: string;
  disabled?: boolean;
}

export function LabelledField({
  field,
  onChange,
}: {
  field: FieldSpec;
  onChange: (value: string) => void;
}) {
  return (
    <div className="flex flex-col gap-1.5">
      <Label htmlFor={field.id}>{field.label}</Label>
      <Input
        id={field.id}
        type={field.type}
        value={field.value}
        disabled={field.disabled}
        autoComplete="off"
        onChange={(e) => onChange(e.target.value)}
      />
      {field.hint && (
        <p className="text-xs text-muted-foreground">{field.hint}</p>
      )}
    </div>
  );
}
