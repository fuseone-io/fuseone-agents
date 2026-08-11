import type { ReactNode } from "react";
import { ThemeProvider as NextThemes } from "next-themes";

/**
 * Both themes are first class.
 *
 * Two attributes are written because two conventions are in play: the design
 * system's tokens key on `data-theme`, and shadcn's primitives ship `dark:`
 * variants keyed on the `.dark` class. Setting both means neither has to bend,
 * and a component from the registry lands correct in either theme with no
 * override.
 */
export function ThemeProvider({ children }: { children: ReactNode }) {
  return (
    <NextThemes
      attribute={["class", "data-theme"]}
      defaultTheme="system"
      enableSystem
      // Suppressing transitions during the swap avoids every surface on the
      // page animating its colour at once, which reads as a glitch.
      disableTransitionOnChange
    >
      {children}
    </NextThemes>
  );
}
