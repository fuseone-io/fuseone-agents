import type { ReactNode } from "react";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { cn } from "@/lib/utils";

export interface BrandFieldConfig {
  id: string;
  label: string;
  placeholder?: string;
  type?: "url" | "text";
  hint?: string;
  invalid?: boolean;
  /** Machine text, such as a hex colour, reads in mono. */
  mono?: boolean;
}

/**
 * One labelled branding input. `children` sits beside the input on the same
 * row — the colour picker, for instance — and `footer` below the hint.
 */
export function BrandField({
  field,
  value,
  onChange,
  children,
  footer,
}: {
  field: BrandFieldConfig;
  value: string;
  onChange: (value: string) => void;
  children?: ReactNode;
  footer?: ReactNode;
}) {
  const { id, label, placeholder, type = "url", hint, invalid, mono } =
    field;
  return (
    <div className="flex flex-col gap-1.5">
      <Label htmlFor={id}>{label}</Label>
      <div className="flex items-center gap-2">
        <Input
          id={id}
          type={type}
          value={value}
          placeholder={placeholder}
          autoComplete="off"
          aria-invalid={invalid || undefined}
          className={mono ? "font-mono" : undefined}
          onChange={(e) => onChange(e.target.value)}
        />
        {children}
      </div>
      {hint && (
        <p
          className={cn(
            "text-xs",
            invalid ? "text-destructive" : "text-muted-foreground",
          )}
        >
          {hint}
        </p>
      )}
      {footer}
    </div>
  );
}
