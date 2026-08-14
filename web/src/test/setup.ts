import "@testing-library/jest-dom/vitest";
import { setLocale } from "@/i18n";

/**
 * Pinned, because otherwise the suite reads whatever language the machine
 * running it prefers: green on a Brazilian laptop and red in CI, over a
 * difference that has nothing to do with the code under test.
 *
 * Portuguese because that is the language the assertions are written in. A
 * test that asserted through the catalogue would pass whenever the catalogue
 * and the component agree, including when they agree on the wrong words.
 */
setLocale("pt-BR");

/**
 * jsdom has no matchMedia, and anything that reads the theme asks for it —
 * which is every screen once a toast is on it. Stubbed as "no preference
 * stated", which is what a browser answers before somebody chooses.
 */
if (!window.matchMedia) {
  window.matchMedia = (query: string) =>
    ({
      matches: false,
      media: query,
      onchange: null,
      addEventListener: () => {},
      removeEventListener: () => {},
      addListener: () => {},
      removeListener: () => {},
      dispatchEvent: () => false,
    }) as MediaQueryList;
}

/**
 * jsdom has no ResizeObserver, and XYFlow measures every node through one —
 * it keeps them hidden and drops handle-bound edges until it has. A stub that
 * observes nothing is enough here: these tests assert what a diagram is built
 * from, never what it measured to.
 */
if (!globalThis.ResizeObserver) {
  globalThis.ResizeObserver = class {
    observe() {}
    unobserve() {}
    disconnect() {}
  };
}

