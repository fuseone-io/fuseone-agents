import js from "@eslint/js";
import globals from "globals";
import reactHooks from "eslint-plugin-react-hooks";
import reactRefresh from "eslint-plugin-react-refresh";
import i18next from "eslint-plugin-i18next";
import tseslint from "typescript-eslint";

export default tseslint.config(
  { ignores: ["dist", "src/lib/api/schema.gen.ts"] },
  {
    extends: [js.configs.recommended, ...tseslint.configs.recommended],
    files: ["**/*.{ts,tsx}"],
    languageOptions: { ecmaVersion: 2022, globals: globals.browser },
    plugins: {
      "react-hooks": reactHooks,
      "react-refresh": reactRefresh,
      i18next,
    },
    rules: {
      ...reactHooks.configs.recommended.rules,
      "react-refresh/only-export-components": ["warn", { allowConstantExport: true }],
      // The core rules say what this codebase has already decided in CLAUDE.md.
      "@typescript-eslint/no-explicit-any": "error",
      "no-restricted-imports": ["error", { patterns: ["../*"] }],
      // A literal in JSX is a bug: it ships in one language to everyone.
      // Whoever adds one is reading their own locale and cannot see the
      // failure, so the rule has to be the thing that notices.
      "i18next/no-literal-string": [
        "error",
        { mode: "jsx-text-only", "should-validate-template": true },
      ],
    },
  },
  {
    // A test asserts on the words a person sees. Routing those through the
    // catalogue would make the test pass whenever the catalogue and the
    // component agree — including when they agree on the wrong words.
    files: ["**/__tests__/**"],
    rules: { "i18next/no-literal-string": "off" },
  },
  {
    // CLI-owned: these files come from `shadcn add` and are re-generated. The
    // findings here are shadcn's own (a random skeleton width, a setState in
    // the mobile-breakpoint effect), and patching them would have to be
    // re-applied on every update for no product gain.
    files: ["src/components/ui/**", "src/hooks/use-mobile.ts"],
    rules: {
      "i18next/no-literal-string": "off",
      "react-hooks/purity": "off",
      "react-hooks/set-state-in-effect": "off",
      "react-refresh/only-export-components": "off",
    },
  },
);
