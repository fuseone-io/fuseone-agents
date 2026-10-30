/*
 * Not from the shadcn CLI: the registry has no colour picker, and Radix has
 * no primitive for one. This wraps the browser's own <input type="color">,
 * which is keyboard-operable and opens the platform's accessible colour
 * dialog — better than a hand-built swatch grid would be. It lives here so
 * no feature folder renders a raw <input>.
 *
 * The value is always "#rrggbb" in lowercase; that is the only form the
 * native control accepts and the only one it reports.
 */
import * as React from "react";

import { cn } from "@/lib/utils";

function ColorInput({
  className,
  ...props
}: Omit<React.ComponentProps<"input">, "type">) {
  return (
    <input
      type="color"
      data-slot="color-input"
      className={cn(
        "h-9 w-12 shrink-0 cursor-pointer rounded-md border border-input bg-transparent p-1 shadow-xs outline-none",
        "focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50",
        "disabled:pointer-events-none disabled:cursor-not-allowed disabled:opacity-50",
        className,
      )}
      {...props}
    />
  );
}

export { ColorInput };
