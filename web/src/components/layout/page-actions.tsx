import {
  createContext,
  useCallback,
  useContext,
  useState,
  type ReactNode,
} from "react";
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
interface Slots {
  actions: HTMLElement | null;
  identity: HTMLElement | null;
  attach: (which: "actions" | "identity", node: HTMLElement | null) => void;
}

const SlotContext = createContext<Slots>({
  actions: null,
  identity: null,
  attach: () => {},
});

/** Holds the slots for everything inside the shell. */
export function PageActionsProvider({ children }: { children: ReactNode }) {
  const [slots, setSlots] = useState<{
    actions: HTMLElement | null;
    identity: HTMLElement | null;
  }>({ actions: null, identity: null });

  const attach = useCallback(
    (which: "actions" | "identity", node: HTMLElement | null) =>
      setSlots((held) => (held[which] === node ? held : { ...held, [which]: node })),
    [],
  );

  return (
    <SlotContext.Provider value={{ ...slots, attach }}>
      {children}
    </SlotContext.Provider>
  );
}

/** Marks the place in the header where the action lands. */
export function PageActionsTarget() {
  return <Target which="actions" className="flex items-center gap-2" />;
}

/**
 * Marks the place beside the breadcrumb where a screen says which record it
 * is on — a name, a version, a state.
 *
 * Beside the crumb rather than under it, because a screen that repeats the
 * header underneath it has two headers: the trail says where you are and this
 * says which one, and they are one line.
 */
export function PageIdentityTarget() {
  return <Target which="identity" className="flex min-w-0 items-center gap-2" />;
}

/**
 * The span a portal lands in.
 *
 * The ref callback is memoised, and that is not a micro-optimisation: a fresh
 * function every render makes React detach and reattach the node, which sets
 * state, which renders again. The screen stops at React's update-depth guard.
 */
function Target({
  which,
  className,
}: {
  which: "actions" | "identity";
  className: string;
}) {
  const { attach } = useContext(SlotContext);
  const hold = useCallback(
    (node: HTMLElement | null) => attach(which, node),
    [attach, which],
  );
  return <span ref={hold} className={className} />;
}

/** Renders into the header's slot, or in place when there is none. */
export function PageActions({ children }: { children: ReactNode }) {
  const { actions } = useContext(SlotContext);
  if (!actions) return <>{children}</>;
  return createPortal(children, actions);
}

/** Renders beside the breadcrumb, or in place when there is no shell. */
export function PageIdentity({ children }: { children: ReactNode }) {
  const { identity } = useContext(SlotContext);
  if (!identity) return <>{children}</>;
  return createPortal(children, identity);
}
