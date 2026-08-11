import js from "@eslint/js";
import globals from "globals";
import reactHooks from "eslint-plugin-react-hooks";
import reactRefresh from "eslint-plugin-react-refresh";
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
    },
    rules: {
      ...reactHooks.configs.recommended.rules,
      "react-refresh/only-export-components": ["warn", { allowConstantExport: true }],
      // The core rules say what this codebase has already decided in CLAUDE.md.
      "@typescript-eslint/no-explicit-any": "error",
      "no-restricted-imports": ["error", { patterns: ["../*"] }],
    },
  },
  {
    // CLI-owned: these files come from `shadcn add` and are re-generated. The
    // findings here are shadcn's own (a random skeleton width, a setState in
    // the mobile-breakpoint effect), and patching them would have to be
    // re-applied on every update for no product gain.
    files: ["src/components/ui/**", "src/hooks/use-mobile.ts"],
    rules: {
      "react-hooks/purity": "off",
      "react-hooks/set-state-in-effect": "off",
      "react-refresh/only-export-components": "off",
    },
  },
);
