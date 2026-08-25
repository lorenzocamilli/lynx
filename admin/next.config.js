// @ts-check

const isExport = process.env.NEXT_EXPORT === "1";

/**
 * @type {import('next').NextConfig}
 **/
const nextConfig = {
  reactStrictMode: true,
  trailingSlash: true,
  // This repo maintains its own (git-excluded) root CLAUDE.md — don't let
  // `next dev` regenerate admin/AGENTS.md + admin/CLAUDE.md alongside it.
  agentRules: false,
  ...(isExport
    ? { output: "export", distDir: "dist" }
    : {
        async rewrites() {
          return [
            {
              source: "/api/events",
              destination: "http://localhost:8080/api/events",
            },
            {
              source: "/api/:path/",
              destination: "http://localhost:8080/api/:path/",
            },
          ];
        },
      }),
};

module.exports = nextConfig;
