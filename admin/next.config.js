// @ts-check

const isExport = process.env.NEXT_EXPORT === "1";

/**
 * @type {import('next').NextConfig}
 **/
const nextConfig = {
  reactStrictMode: true,
  trailingSlash: true,
  ...(isExport
    ? { output: "export", distDir: "dist" }
    : {
        async rewrites() {
          return [
            {
              source: "/api/:path/",
              destination: "http://localhost:8080/api/:path/",
            },
          ];
        },
      }),
};

module.exports = nextConfig;
