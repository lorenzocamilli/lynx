import { useApolloClient } from "@apollo/client";
import ContentCopyIcon from "@mui/icons-material/ContentCopy";
import { TabContext, TabPanel } from "@mui/lab";
import TabList from "@mui/lab/TabList";
import {
  Alert,
  Box,
  Button,
  CircularProgress,
  FormControl,
  FormHelperText,
  IconButton,
  InputAdornment,
  MenuItem,
  Snackbar,
  Tab,
  TextField,
  TextFieldProps,
  Tooltip,
  Typography,
} from "@mui/material";
import MaterialLink from "@mui/material/Link";
import { useEffect, useState } from "react";

import { useActiveProject } from "lib/ActiveProjectContext";
import { authedFetch, getToken } from "lib/auth";
import Link from "lib/components/Link";
import { ActiveProjectDocument, useUpdateInterceptSettingsMutation } from "lib/graphql/generated";
import { withoutTypename } from "lib/graphql/omitTypename";

enum TabValue {
  Intercept = "intercept",
  Application = "application",
}

interface AppConfig {
  host: string;
  port: number;
  logLevel: string;
}

const logLevels = ["debug", "info", "warn", "error"];

function FilterTextField(props: TextFieldProps): JSX.Element {
  return (
    <TextField
      color="primary"
      variant="outlined"
      InputProps={{
        sx: { fontFamily: "'JetBrains Mono', monospace" },
        autoCorrect: "false",
        spellCheck: "false",
      }}
      InputLabelProps={{
        shrink: true,
      }}
      margin="normal"
      sx={{ mr: 1 }}
      {...props}
    />
  );
}

export default function Settings(): JSX.Element {
  const client = useApolloClient();
  const activeProject = useActiveProject();
  const [updateInterceptSettings, updateIntercepSettingsResult] = useUpdateInterceptSettingsMutation({
    onCompleted(data) {
      client.cache.updateQuery({ query: ActiveProjectDocument }, (cachedData) => ({
        activeProject: {
          ...cachedData.activeProject,
          settings: {
            ...cachedData.activeProject.settings,
            intercept: data.updateInterceptSettings,
          },
        },
      }));

      setInterceptReqFilter(data.updateInterceptSettings.requestFilter || "");
      setInterceptResFilter(data.updateInterceptSettings.responseFilter || "");
      setSettingsUpdatedOpen(true);
    },
  });

  const [interceptReqFilter, setInterceptReqFilter] = useState("");
  const [interceptResFilter, setInterceptResFilter] = useState("");

  useEffect(() => {
    setInterceptReqFilter(activeProject?.settings.intercept.requestFilter || "");
  }, [activeProject?.settings.intercept.requestFilter]);

  useEffect(() => {
    setInterceptResFilter(activeProject?.settings.intercept.responseFilter || "");
  }, [activeProject?.settings.intercept.responseFilter]);

  const handleInterceptReqFilter = () => {
    if (!activeProject) return;
    updateInterceptSettings({
      variables: {
        input: {
          ...withoutTypename(activeProject.settings.intercept),
          requestFilter: interceptReqFilter,
        },
      },
    });
  };

  const handleInterceptResFilter = () => {
    if (!activeProject) return;
    updateInterceptSettings({
      variables: {
        input: {
          ...withoutTypename(activeProject.settings.intercept),
          responseFilter: interceptResFilter,
        },
      },
    });
  };

  // Application config state.
  const [appConfig, setAppConfig] = useState<AppConfig>({ host: "127.0.0.1", port: 8080, logLevel: "info" });
  const [appConfigLoading, setAppConfigLoading] = useState(false);
  const [appConfigSaving, setAppConfigSaving] = useState(false);
  const [restartRequired, setRestartRequired] = useState(false);
  const [appConfigError, setAppConfigError] = useState<string | null>(null);

  useEffect(() => {
    setAppConfigLoading(true);
    authedFetch("/api/settings")
      .then((res) => res.json())
      .then((data: AppConfig) => setAppConfig(data))
      .catch(() => setAppConfigError("Failed to load application settings."))
      .finally(() => setAppConfigLoading(false));
  }, []);

  const handleAppConfigSave = () => {
    setAppConfigSaving(true);
    setAppConfigError(null);
    authedFetch("/api/settings", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(appConfig),
    })
      .then((res) => {
        if (!res.ok) throw new Error("Server error");
        setRestartRequired(true);
      })
      .catch(() => setAppConfigError("Failed to save application settings."))
      .finally(() => setAppConfigSaving(false));
  };

  const [adminToken, setAdminToken] = useState("");
  const [tokenCopied, setTokenCopied] = useState(false);
  useEffect(() => {
    getToken().then(setAdminToken);
  }, []);
  const handleCopyToken = async () => {
    await navigator.clipboard.writeText(adminToken);
    setTokenCopied(true);
    setTimeout(() => setTokenCopied(false), 2000);
  };

  const [tabValue, setTabValue] = useState(TabValue.Intercept);
  const [settingsUpdatedOpen, setSettingsUpdatedOpen] = useState(false);

  const handleSettingsUpdatedClose = (_: Event | React.SyntheticEvent, reason?: string) => {
    if (reason === "clickaway") return;
    setSettingsUpdatedOpen(false);
  };

  const tabSx = { textTransform: "none" };

  return (
    <Box p={4}>
      <Snackbar open={settingsUpdatedOpen} autoHideDuration={3000} onClose={handleSettingsUpdatedClose}>
        <Alert onClose={handleSettingsUpdatedClose} severity="info">
          Intercept settings have been updated.
        </Alert>
      </Snackbar>

      <Typography variant="h4" sx={{ mb: 2 }}>
        Settings
      </Typography>
      <Typography paragraph sx={{ mb: 4 }}>
        Settings allow you to tweak the behaviour of Lynx&apos;s features.
      </Typography>

      <TabContext value={tabValue}>
        <TabList onChange={(_, value) => setTabValue(value)} sx={{ borderBottom: 1, borderColor: "divider" }}>
          <Tab value={TabValue.Intercept} label="Intercept" sx={tabSx} />
          <Tab value={TabValue.Application} label="Application" sx={tabSx} />
        </TabList>

        <TabPanel value={TabValue.Intercept} sx={{ px: 0 }}>
          <Typography variant="h5" sx={{ mb: 2 }}>
            Project settings
          </Typography>
          {!activeProject && (
            <Typography paragraph>
              There is no project active. To configure project settings, first{" "}
              <Link href="/projects">open a project</Link>.
            </Typography>
          )}
          {activeProject && (
            <>
              <Typography variant="h6" sx={{ mt: 3, mb: 1 }}>
                Request filter
              </Typography>
              <form>
                <FormControl sx={{ width: "50%" }}>
                  <FilterTextField
                    label="Request filter"
                    placeholder={`Example: method = "GET" OR url =~ "/foobar"`}
                    value={interceptReqFilter}
                    onChange={(e) => setInterceptReqFilter(e.target.value)}
                  />
                  <FormHelperText>
                    Filter expression to match incoming requests on. When set, only matching requests are intercepted.{" "}
                    <MaterialLink href="https://github.com/lorenzocamilli/lynx" target="_blank">
                      Read docs.
                    </MaterialLink>
                  </FormHelperText>
                </FormControl>
                <Button
                  type="submit"
                  variant="text"
                  color="primary"
                  size="large"
                  sx={{ mt: 2, py: 1.8 }}
                  onClick={handleInterceptReqFilter}
                  disabled={updateIntercepSettingsResult.loading}
                  startIcon={updateIntercepSettingsResult.loading ? <CircularProgress size={22} /> : undefined}
                >
                  Update
                </Button>
              </form>

              <Typography variant="h6" sx={{ mt: 3, mb: 1 }}>
                Response filter
              </Typography>
              <form>
                <FormControl sx={{ width: "50%" }}>
                  <FilterTextField
                    label="Response filter"
                    placeholder={`Example: statusCode =~ "^2" OR body =~ "foobar"`}
                    value={interceptResFilter}
                    onChange={(e) => setInterceptResFilter(e.target.value)}
                  />
                  <FormHelperText>
                    Filter expression to match received responses on. When set, only matching responses are intercepted.{" "}
                    <MaterialLink href="https://github.com/lorenzocamilli/lynx" target="_blank">
                      Read docs.
                    </MaterialLink>
                  </FormHelperText>
                </FormControl>
                <Button
                  type="submit"
                  variant="text"
                  color="primary"
                  size="large"
                  sx={{ mt: 2, py: 1.8 }}
                  onClick={handleInterceptResFilter}
                  disabled={updateIntercepSettingsResult.loading}
                  startIcon={updateIntercepSettingsResult.loading ? <CircularProgress size={22} /> : undefined}
                >
                  Update
                </Button>
              </form>
            </>
          )}
        </TabPanel>

        <TabPanel value={TabValue.Application} sx={{ px: 0 }}>
          <Typography variant="h5" sx={{ mb: 2 }}>
            Application settings
          </Typography>
          <Typography paragraph color="text.secondary" sx={{ mb: 3 }}>
            Changes to these settings take effect after restarting Lynx.
          </Typography>

          {restartRequired && (
            <Alert severity="warning" sx={{ mb: 3 }} onClose={() => setRestartRequired(false)}>
              Settings saved. Restart Lynx for the changes to take effect.
            </Alert>
          )}

          {appConfigError && (
            <Alert severity="error" sx={{ mb: 3 }} onClose={() => setAppConfigError(null)}>
              {appConfigError}
            </Alert>
          )}

          <Typography variant="h6" sx={{ mt: 3, mb: 1 }}>
            Admin token
          </Typography>
          <Typography paragraph color="text.secondary" sx={{ mb: 2 }}>
            Required for all API requests. Stored in <code>~/.lynx/token</code> and printed to the terminal at startup.
          </Typography>
          <Box sx={{ maxWidth: 480, mb: 4 }}>
            <TextField
              label="Access token"
              value={adminToken}
              variant="outlined"
              fullWidth
              slotProps={{
                input: {
                  readOnly: true,
                  sx: { fontFamily: "var(--font-code)", fontSize: "0.8rem" },
                  endAdornment: (
                    <InputAdornment position="end">
                      <Tooltip title={tokenCopied ? "Copied!" : "Copy token"}>
                        <IconButton onClick={handleCopyToken} edge="end">
                          <ContentCopyIcon fontSize="small" />
                        </IconButton>
                      </Tooltip>
                    </InputAdornment>
                  ),
                },
              }}
            />
          </Box>

          <Typography variant="h6" sx={{ mb: 1 }}>
            Network
          </Typography>

          {appConfigLoading ? (
            <CircularProgress />
          ) : (
            <Box component="form" sx={{ display: "flex", flexDirection: "column", maxWidth: 480, gap: 2 }}>
              <TextField
                label="Listen address"
                helperText='Host the proxy binds to. Use "127.0.0.1" (default, localhost only) or "0.0.0.0" to expose on all interfaces.'
                value={appConfig.host}
                onChange={(e) => setAppConfig((c) => ({ ...c, host: e.target.value }))}
                slotProps={{ inputLabel: { shrink: true } }}
                variant="outlined"
                fullWidth
              />
              {appConfig.host === "0.0.0.0" && (
                <Alert severity="warning" sx={{ mt: -1 }}>
                  Binding to 0.0.0.0 exposes the admin UI and captured traffic to your entire network.
                </Alert>
              )}
              <TextField
                label="Listen port"
                helperText="Port the proxy listens on (1–65535)."
                type="number"
                value={appConfig.port}
                onChange={(e) => setAppConfig((c) => ({ ...c, port: parseInt(e.target.value, 10) }))}
                slotProps={{ inputLabel: { shrink: true }, htmlInput: { min: 1, max: 65535 } }}
                variant="outlined"
                fullWidth
              />
              <TextField
                select
                label="Log level"
                helperText="Minimum severity written to the server log."
                value={appConfig.logLevel ?? "info"}
                onChange={(e) => setAppConfig((c) => ({ ...c, logLevel: e.target.value }))}
                slotProps={{ inputLabel: { shrink: true } }}
                variant="outlined"
                fullWidth
              >
                {logLevels.map((level) => (
                  <MenuItem key={level} value={level}>
                    {level}
                  </MenuItem>
                ))}
              </TextField>
              <Box>
                <Button
                  variant="contained"
                  color="primary"
                  onClick={handleAppConfigSave}
                  disabled={appConfigSaving}
                  startIcon={appConfigSaving ? <CircularProgress size={18} /> : undefined}
                >
                  Save settings
                </Button>
              </Box>
            </Box>
          )}
        </TabPanel>
      </TabContext>
    </Box>
  );
}
