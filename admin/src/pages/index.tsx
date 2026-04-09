import DownloadIcon from "@mui/icons-material/Download";
import FolderIcon from "@mui/icons-material/Folder";
import { Box, Button, Typography } from "@mui/material";
import Link from "next/link";

import { Layout, Page } from "features/Layout";

function Index(): JSX.Element {
  const highlightSx = { color: "primary.main" };

  return (
    <Layout page={Page.Home} title="">
      <Box p={4}>
        <Box mb={4} width="60%">
          <Typography variant="h2">
            <Box component="span" sx={highlightSx}>
              Hetty://
            </Box>
            <br />
            The simple HTTP toolkit for security research.
          </Typography>
        </Box>

        <Typography
          paragraph
          sx={{
            fontSize: "1.6rem",
            width: "60%",
            lineHeight: 2,
            mb: 5,
          }}
        >
          Welcome to{" "}
          <Box component="span" sx={highlightSx}>
            Hetty
          </Box>
          . Get started by creating a project.
        </Typography>

        <Link href="/projects" passHref>
          <Button
            sx={{ mr: 2 }}
            variant="contained"
            color="primary"
            component="a"
            size="large"
            startIcon={<FolderIcon />}
          >
            Manage projects
          </Button>
        </Link>

        <Button
          variant="outlined"
          color="primary"
          size="large"
          startIcon={<DownloadIcon />}
          component="a"
          href="/api/ca.crt"
        >
          Download CA certificate
        </Button>
      </Box>
    </Layout>
  );
}

export default Index;
