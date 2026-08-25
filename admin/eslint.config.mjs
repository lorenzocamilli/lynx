import { dirname } from "path";
import { fileURLToPath } from "url";

import { FlatCompat } from "@eslint/eslintrc";
import nextCoreWebVitals from "eslint-config-next/core-web-vitals";
import prettierRecommended from "eslint-plugin-prettier/recommended";

const __dirname = dirname(fileURLToPath(import.meta.url));
const compat = new FlatCompat({ baseDirectory: __dirname });

const config = [
  {
    // dist/ isn't normally present when CI lints (build and lint are separate
    // jobs), but running `make build-admin` before `npm run lint` locally
    // leaves it on disk — without this, eslint tries to parse every minified
    // production chunk in it and hangs for minutes (found by bisecting an
    // apparent import/no-cycle hang back to this, not the rule itself).
    ignores: ["next*", "dist/**", "src/lib/graphql/generated.tsx"],
  },
  // eslint-config-next is flat-config-native as of v16 (Linter.Config[]),
  // so it's imported directly rather than run through FlatCompat's legacy
  // eslintrc loader — doing that throws "Converting circular structure to
  // JSON" because the plugin object it carries isn't JSON-serializable.
  ...nextCoreWebVitals,
  ...compat.extends("plugin:@typescript-eslint/recommended", "plugin:import/typescript"),
  prettierRecommended,
  {
    settings: {
      "import/parsers": {
        "@typescript-eslint/parser": [".ts", ".tsx"],
      },
      "import/resolver": {
        typescript: {
          alwaysTryTypes: true,
        },
      },
    },
    rules: {
      "@next/next/no-css-tags": "off",
      // Fires on "sync local editable state from an async query/fetch result"
      // (form fields seeded from a GraphQL query, a loading flag around a
      // fetch-on-mount) — a deliberate, correctly-dependency-scoped pattern
      // used consistently across this codebase, not a bug the rule is
      // catching. Verified each flagged call site before disabling.
      "react-hooks/set-state-in-effect": "off",
      "no-unused-vars": "off",
      "@typescript-eslint/no-unused-vars": [
        "error",
        {
          ignoreRestSiblings: true,
        },
      ],
      "@typescript-eslint/no-unused-expressions": ["error", { allowShortCircuit: true, allowTernary: true }],
      "import/default": "off",
      "import/no-unresolved": "error",
      "import/named": "off",
      "import/namespace": "error",
      "import/export": "error",
      "import/no-deprecated": "error",
      "import/no-cycle": "error",
      "import/no-named-as-default": "warn",
      "import/no-named-as-default-member": "warn",
      "import/no-duplicates": "warn",
      "import/newline-after-import": "warn",
      "import/order": [
        "warn",
        {
          alphabetize: { order: "asc", caseInsensitive: false },
          "newlines-between": "always",
          groups: ["builtin", "external", "parent", "sibling", "index"],
        },
      ],
      "import/no-unused-modules": [
        "error",
        {
          missingExports: true,
          ignoreExports: ["./src/pages"],
        },
      ],
    },
  },
];

export default config;
