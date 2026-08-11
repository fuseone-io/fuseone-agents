import type { ReactNode } from "react";
import { cn } from "@/lib/utils";

/**
 * Anything the platform produced rather than a person wrote.
 *
 * Run ids, costs, latencies, policy codes, hashes. The sans/mono split is the
 * design system's strongest signal about provenance, and the fixed-width
 * digits are what make a column of numbers scannable instead of decorative.
 */
export function Mono({
  children,
  dim,
  className,
}: {
  children: ReactNode;
  dim?: boolean;
  className?: string;
}) {
  return (
    <span
      className={cn(
        "font-mono text-xs tabular-nums",
        dim && "text-muted-foreground",
        className,
      )}
    >
      {children}
    </span>
  );
}
