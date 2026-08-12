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
