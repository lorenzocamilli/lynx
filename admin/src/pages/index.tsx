import AltRouteIcon from "@mui/icons-material/AltRoute";
import DownloadIcon from "@mui/icons-material/Download";
import FolderOpenIcon from "@mui/icons-material/FolderOpen";
import LocationSearchingIcon from "@mui/icons-material/LocationSearching";
import PauseCircleOutlineIcon from "@mui/icons-material/PauseCircleOutline";
import SendIcon from "@mui/icons-material/Send";
import { Box, Button, Chip, Divider, Grid, Paper, Typography } from "@mui/material";

import { Layout, Page } from "features/Layout";
import Link from "lib/components/Link";

interface FeatureCardProps {
  icon: React.ReactNode;
  title: string;
  description: string;
  href: string;
  label: string;
}

function FeatureCard({ icon, title, description, href, label }: FeatureCardProps): JSX.Element {
  return (
    <Paper
      sx={{
        p: 3,
        height: "100%",
        display: "flex",
        flexDirection: "column",
        gap: 1.5,
        border: "1px solid",
        borderColor: "secondary.light",
        transition: "border-color 0.15s ease",
        "&:hover": { borderColor: "primary.main" },
      }}
    >
      <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
        <Box sx={{ color: "primary.main", display: "flex" }}>{icon}</Box>
        <Typography variant="h6" component="h3">
          {title}
        </Typography>
      </Box>
      <Typography variant="body2" color="text.secondary" sx={{ flexGrow: 1 }}>
        {description}
      </Typography>
      <Button
        component={Link}
        href={href}
        variant="text"
        color="primary"
        size="small"
        sx={{ alignSelf: "flex-start", px: 0 }}
      >
        {label} →
      </Button>
    </Paper>
  );
}

function Index(): JSX.Element {
  return (
    <Layout page={Page.Home} title="">
      <Box sx={{ maxWidth: 960, mx: "auto", px: 4, py: 6 }}>
        {/* Hero */}
        <Box sx={{ mb: 6 }}>
          <Chip
            label="HTTP security toolkit"
            size="small"
            sx={{
              mb: 2,
              bgcolor: "secondary.main",
              color: "primary.main",
              border: "1px solid",
              borderColor: "primary.main",
              fontFamily: "'Space Grotesk', sans-serif",
              fontWeight: 600,
              letterSpacing: 0.5,
            }}
          />
          <Typography
            variant="h2"
            sx={{
              fontFamily: "'Space Grotesk', sans-serif",
              fontWeight: 700,
              fontSize: { xs: "2.4rem", md: "3.2rem" },
              lineHeight: 1.15,
              mb: 2,
            }}
          >
            <Box component="span" sx={{ color: "primary.main" }}>
              Lynx
            </Box>{" "}
            — intercept, inspect,
            <br />
            and replay HTTP traffic.
          </Typography>
          <Typography
            variant="body1"
            color="text.secondary"
            sx={{ fontSize: "1.1rem", maxWidth: 560, lineHeight: 1.8, mb: 4 }}
          >
            A self-hosted MITM proxy and HTTP toolkit for security research. Open source, runs locally, no telemetry.
          </Typography>
          <Box sx={{ display: "flex", gap: 2, flexWrap: "wrap" }}>
            <Button
              component={Link}
              href="/projects"
              variant="contained"
              color="primary"
              size="large"
              startIcon={<FolderOpenIcon />}
              sx={{ fontWeight: 600 }}
            >
              Open a project
            </Button>
            <Button
              variant="outlined"
              color="primary"
              size="large"
              startIcon={<DownloadIcon />}
              component="a"
              href="/api/ca.crt"
              sx={{ fontWeight: 600 }}
            >
              Download CA cert
            </Button>
          </Box>
        </Box>

        <Divider sx={{ borderColor: "secondary.light", mb: 6 }} />

        {/* Features */}
        <Box sx={{ mb: 2 }}>
          <Typography variant="h5" sx={{ mb: 0.5 }}>
            Features
          </Typography>
          <Typography variant="body2" color="text.secondary" sx={{ mb: 3 }}>
            Everything runs in your browser. No cloud, no accounts.
          </Typography>
          <Grid container spacing={2}>
            <Grid item xs={12} sm={6}>
              <FeatureCard
                icon={<AltRouteIcon />}
                title="Proxy logs"
                description="Full HTTP/HTTPS traffic log with search and filter. Inspect headers and bodies for every request your browser makes."
                href="/proxy/logs"
                label="Open logs"
              />
            </Grid>
            <Grid item xs={12} sm={6}>
              <FeatureCard
                icon={<PauseCircleOutlineIcon />}
                title="Intercept"
                description="Pause requests and responses mid-flight, edit them in the browser, then forward or drop. Essential for manual testing."
                href="/proxy/intercept"
                label="Open intercept"
              />
            </Grid>
            <Grid item xs={12} sm={6}>
              <FeatureCard
                icon={<SendIcon />}
                title="Sender"
                description="Craft and replay arbitrary HTTP requests. Import from proxy logs, tweak headers and bodies, send, repeat."
                href="/sender"
                label="Open sender"
              />
            </Grid>
            <Grid item xs={12} sm={6}>
              <FeatureCard
                icon={<LocationSearchingIcon />}
                title="Scope"
                description="Regex-based scope rules to filter which hosts and paths are logged or intercepted. Keep your session focused."
                href="/scope"
                label="Configure scope"
              />
            </Grid>
          </Grid>
        </Box>

        <Divider sx={{ borderColor: "secondary.light", my: 6 }} />

        {/* Setup steps */}
        <Box>
          <Typography variant="h5" sx={{ mb: 3 }}>
            Getting started
          </Typography>
          <Box sx={{ display: "flex", flexDirection: "column", gap: 2 }}>
            {[
              {
                step: "01",
                title: "Create a project",
                body: "All proxy data is scoped to a project. Create one to start capturing traffic.",
                href: "/projects",
                cta: "Go to projects",
              },
              {
                step: "02",
                title: "Install the CA certificate",
                body: "Download and trust the Lynx root CA so HTTPS traffic is visible in plain text.",
                href: "/api/ca.crt",
                cta: "Download cert",
                external: true,
              },
              {
                step: "03",
                title: "Set your browser proxy",
                body: "Point your browser's HTTP/HTTPS proxy to 127.0.0.1:8080 and start browsing.",
              },
            ].map(({ step, title, body, href, cta, external }) => (
              <Box
                key={step}
                sx={{
                  display: "flex",
                  gap: 3,
                  alignItems: "flex-start",
                  p: 2.5,
                  borderRadius: 1,
                  border: "1px solid",
                  borderColor: "secondary.light",
                }}
              >
                <Typography
                  sx={{
                    fontFamily: "'Space Grotesk', sans-serif",
                    fontWeight: 700,
                    fontSize: "1.4rem",
                    color: "primary.main",
                    lineHeight: 1,
                    minWidth: 32,
                    mt: 0.25,
                  }}
                >
                  {step}
                </Typography>
                <Box sx={{ flexGrow: 1 }}>
                  <Typography variant="subtitle1" sx={{ fontWeight: 600, mb: 0.5 }}>
                    {title}
                  </Typography>
                  <Typography variant="body2" color="text.secondary">
                    {body}
                  </Typography>
                </Box>
                {href && cta && (
                  <Button
                    // /api/ca.crt isn't a Next.js page route, so it must stay a
                    // plain anchor rather than going through next/link's client
                    // routing; internal steps (e.g. /projects) use the Link
                    // wrapper instead.
                    component={external ? "a" : Link}
                    href={href}
                    variant="outlined"
                    color="primary"
                    size="small"
                    {...(external ? { target: "_blank" } : {})}
                    sx={{ whiteSpace: "nowrap", flexShrink: 0 }}
                  >
                    {cta}
                  </Button>
                )}
              </Box>
            ))}
          </Box>
        </Box>
      </Box>
    </Layout>
  );
}

export default Index;
