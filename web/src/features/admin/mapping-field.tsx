import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

export function MappingField({
  id,
  label,
  value,
  onChange,
}: {
  id: string;
  label: string;
  value: string;
  onChange: (value: string) => void;
}) {
  return (
    <div className="flex flex-col gap-1.5">
      <Label htmlFor={id} className="text-xs">
        {label}
      </Label>
      <Input
        id={id}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        className="h-9 font-mono text-xs"
        autoComplete="off"
      />
    </div>
  );
}
