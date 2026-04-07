import { FlatCompat } from "@eslint/eslintrc";
import { dirname } from "path";
import { fileURLToPath } from "url";
import prettierRecommended from "eslint-plugin-prettier/recommended";

const __dirname = dirname(fileURLToPath(import.meta.url));
const compat = new FlatCompat({ baseDirectory: __dirname });

export default [
  {
    ignores: ["next*", "src/lib/graphql/generated.tsx"],
  },
  ...compat.extends(
    "next/core-web-vitals",
    "plugin:@typescript-eslint/recommended",
    "plugin:import/typescript"
  ),
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
      "no-unused-vars": "off",
      "@typescript-eslint/no-unused-vars": [
        "error",
        {
          ignoreRestSiblings: true,
        },
      ],
      "@typescript-eslint/no-unused-expressions": [
        "error",
        { allowShortCircuit: true, allowTernary: true },
      ],
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
