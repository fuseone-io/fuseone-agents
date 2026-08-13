import { beforeEach, describe, expect, it } from "vitest";
import { usePreferences } from "@/features/preferences/use-preferences";

/**
 * The console remembers the shape of the window.
 *
 * None of this is data and none of it is authority — which is exactly why it
 * has to persist. An operator reloads a run screen dozens of times a day, and
 * a console that re-expands the sidebar and jumps back to the first tab on
 * every one of them is a console that argues with the person using it.
 *
 * This suite runs where `localStorage` does not exist at all, which is not a
 * gap in the harness: it is the locked-down browser profile the console has to
 * survive inside a customer's network. Everything below therefore also asserts
 * that the store works when nothing can be written down.
 */
describe("what the console remembers", () => {
  beforeEach(() => {
    usePreferences.setState({
      sidebarOpen: true,
      tabs: {},
      agentsView: "cards",
    });
  });

  it("keeps the sidebar collapsed once somebody collapses it", () => {
    usePreferences.getState().setSidebarOpen(false);
    expect(usePreferences.getState().sidebarOpen).toBe(false);
  });

  it("keeps each screen's tab apart", () => {
    // One store rather than a key per screen: a screen that invented its own
    // is a screen somebody forgot when clearing them.
    usePreferences.getState().chooseTab("admin", "people");
    usePreferences.getState().chooseTab("integrations", "providers");

    expect(usePreferences.getState().tabs).toEqual({
      admin: "people",
      integrations: "providers",
    });
  });

  it("has nothing to say about a screen nobody has opened", () => {
    // The screen supplies its own fallback, so an unopened one must answer
    // with nothing rather than with somebody else's tab.
    expect(usePreferences.getState().tabs["never-opened"]).toBeUndefined();
  });

  it("remembers cards or a list", () => {
    usePreferences.getState().setAgentsView("list");
    expect(usePreferences.getState().agentsView).toBe("list");
  });
});
