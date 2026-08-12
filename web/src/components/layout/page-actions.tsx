import { createContext, useContext, useState, type ReactNode } from "react";
import { createPortal } from "react-dom";

/**
 * Where a screen's primary action is rendered: the header, not the page.
 *
 * A portal rather than shared state. The alternative — a page publishing its
 * action into a store the header reads — needs an effect to set it and another
 * to clear it, and the window between those two is a header showing the last
 * screen's button. A portal mounts and unmounts with the page that owns it.
 *
 * The slot is optional. A PageHeader rendered without the shell — in a test,
 * or before anybody has signed in — keeps its actions inline rather than
 * dropping them somewhere nobody can see.
 */
const SlotContext = createContext<{
  node: HTMLElement | null;
  attach: (node: HTMLElement | null) => void;
}>({ node: null, attach: () => {} });

/** Holds the slot for everything inside the shell. */
export function PageActionsProvider({ children }: { children: ReactNode }) {
  const [node, attach] = useState<HTMLElement | null>(null);
  return <SlotContext.Provider value={{ node, attach }}>{children}</SlotContext.Provider>;
}

/** Marks the place in the header where the action lands. */
export function PageActionsTarget() {
  const { attach } = useContext(SlotContext);
  return <span ref={attach} className="flex items-center gap-2" />;
}

/** Renders into the header's slot, or in place when there is none. */
export function PageActions({ children }: { children: ReactNode }) {
  const { node } = useContext(SlotContext);
  if (!node) return <>{children}</>;
  return createPortal(children, node);
}
