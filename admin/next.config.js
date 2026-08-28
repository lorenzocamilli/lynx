// @ts-check

const isExport = process.env.NEXT_EXPORT === "1";

/**
 * @type {import('next').NextConfig}
 **/
const nextConfig = {
  reactStrictMode: true,
  trailingSlash: true,
  // Don't let `next dev` auto-generate admin/AGENTS.md (and an
  // admin-scoped agent-instructions file) on every start.
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
