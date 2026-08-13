import { create } from "zustand";
import { createJSONStorage, persist } from "zustand/middleware";
import { durableOrMemory } from "@/lib/durable-storage";

/**
 * What the console remembers about how somebody likes to look at it.
 *
 * Nothing here is data and nothing here is authority: it is the shape of the
 * window. It persists because the alternative is a console that forgets, on
 * every reload, that this person collapses the sidebar and works out of the
 * administration area's people tab — and an operator reloads a run screen
 * dozens of times a day.
 *
 * Deliberately not in the URL. A tab is a preference, not a place: pasting a
 * link to a colleague should take them to the screen, not to whichever corner
 * of it the sender happened to be in.
 */
interface Preferences {
  /** The sidebar is expanded unless somebody collapsed it. */
  sidebarOpen: boolean;
  setSidebarOpen: (open: boolean) => void;

  /** The tab each tabbed screen was left on, keyed by screen. */
  tabs: Record<string, string>;
  chooseTab: (screen: string, tab: string) => void;

  /** Cards or a list, on the screens that offer both. */
  agentsView: string;
  setAgentsView: (view: string) => void;
}

export const usePreferences = create<Preferences>()(
  persist(
    (set) => ({
      sidebarOpen: true,
      setSidebarOpen: (sidebarOpen) => set({ sidebarOpen }),
      tabs: {},
      chooseTab: (screen, tab) =>
        set((state) => ({ tabs: { ...state.tabs, [screen]: tab } })),
      agentsView: "cards",
      setAgentsView: (agentsView) => set({ agentsView }),
    }),
    {
      name: "fuseone.preferences",
      storage: createJSONStorage(durableOrMemory("fuseone.preferences")),
    },
  ),
);

/**
 * The tab a screen opens on, and how to remember a change.
 *
 * A hook rather than three lines in every page, so a screen cannot half-do it:
 * remembering the change and then opening on the default anyway is the bug
 * this shape makes impossible to write.
 */
export function useTab(screen: string, fallback: string) {
  const value = usePreferences((s) => s.tabs[screen] ?? fallback);
  const chooseTab = usePreferences((s) => s.chooseTab);
  return { value, onValueChange: (tab: string) => chooseTab(screen, tab) };
}
