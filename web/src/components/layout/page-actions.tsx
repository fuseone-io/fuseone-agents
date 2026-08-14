import {
  createContext,
  useCallback,
  useContext,
  useEffect,
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
  /** What the last crumb should read, when the record knows its own name. */
  label?: string;
  name: (label?: string) => void;
  /** Whether the chrome around the page should stand back. */
  compact: boolean;
  setCompact: (compact: boolean) => void;
}

const SlotContext = createContext<Slots>({
  actions: null,
  identity: null,
  attach: () => {},
  name: () => {},
  compact: false,
  setCompact: () => {},
});

/**
 * What the shell should call this screen, and how much room it should take.
 *
 * Compact is asked for rather than set: an editor collapsing the navigation by
 * writing to somebody's stored preference leaves it collapsed on the next
 * screen they open, which is a screen quietly changing a setting it was only
 * borrowing. Asked this way there is nothing to put back.
 */
export function useChrome({
  label,
  compact,
}: {
  label?: string;
  compact?: boolean;
}) {
  const { name, setCompact } = useContext(SlotContext);

  useEffect(() => {
    name(label);
    return () => name(undefined);
  }, [name, label]);

  useEffect(() => {
    if (!compact) return;
    setCompact(true);
    return () => setCompact(false);
  }, [setCompact, compact]);
}

/** What the shell reads to draw its own chrome. */
export function useShellChrome() {
  const { label, compact } = useContext(SlotContext);
  return { label, compact };
}

/** Holds the slots for everything inside the shell. */
export function PageActionsProvider({ children }: { children: ReactNode }) {
  const [slots, setSlots] = useState<{
    actions: HTMLElement | null;
    identity: HTMLElement | null;
  }>({ actions: null, identity: null });
  const [label, setLabel] = useState<string | undefined>(undefined);
  const [compact, setCompactState] = useState(false);

  const name = useCallback((next?: string) => setLabel(next), []);
  const setCompact = useCallback((next: boolean) => setCompactState(next), []);

  const attach = useCallback(
    (which: "actions" | "identity", node: HTMLElement | null) =>
      setSlots((held) => (held[which] === node ? held : { ...held, [which]: node })),
    [],
  );

  return (
    <SlotContext.Provider
      value={{ ...slots, attach, label, name, compact, setCompact }}
    >
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
