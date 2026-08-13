/**
 * localStorage where it exists, memory where it does not.
 *
 * The console is installed inside the customer's network and a locked-down
 * browser profile can refuse storage outright. Reading it unguarded throws
 * while the module is still evaluating, which takes down the whole console
 * rather than the one preference it was trying to remember.
 *
 * Shared rather than copied. It was written once for the scope switcher, and a
 * second copy would be a second place for the guard to be forgotten.
 */
export function durableOrMemory(probeKey: string): () => Storage {
  return () => {
    try {
      const probe = globalThis.localStorage;
      if (probe) {
        probe.getItem(probeKey);
        return probe;
      }
    } catch {
      // Blocked. The choice lasts the session instead of surviving a reload.
    }
    return heldInMemory();
  };
}

function heldInMemory(): Storage {
  const held = new Map<string, string>();
  return {
    getItem: (key) => held.get(key) ?? null,
    setItem: (key, value) => void held.set(key, value),
    removeItem: (key) => void held.delete(key),
    clear: () => held.clear(),
    key: (i) => [...held.keys()][i] ?? null,
    get length() {
      return held.size;
    },
  };
}
