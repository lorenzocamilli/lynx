import { createTheme } from "@mui/material/styles";

declare module "@mui/material/Paper" {
  interface PaperPropsVariantOverrides {
    centered: true;
  }
}

const heading = {
  fontFamily: "'Space Grotesk', sans-serif",
  fontWeight: 700,
};

let theme = createTheme({
  palette: {
    mode: "dark",
    primary: {
      main: "#38BDF8",
    },
    secondary: {
      main: "#21262D",
      light: "#30363D",
      dark: "#010409",
    },
    text: {
      primary: "#E6EDF3",
      secondary: "#8B949E",
    },
    background: {
      default: "#0D1117",
      paper: "#161B22",
    },
  },
  typography: {
    fontFamily: "'Inter', sans-serif",
    h2: heading,
    h3: heading,
    h4: heading,
    h5: heading,
    h6: heading,
  },
});

theme = createTheme(theme, {
  palette: {
    info: {
      main: theme.palette.primary.main,
    },
    success: {
      main: theme.palette.primary.main,
    },
  },
  components: {
    MuiTableRow: {
      styleOverrides: {
        root: {
          "&.Mui-selected, &.Mui-selected:hover": {
            backgroundColor: "#21262D",
          },
        },
      },
    },
    MuiPaper: {
      variants: [
        {
          props: { variant: "centered" },
          style: {
            display: "flex",
            justifyContent: "center",
            alignItems: "center",
            padding: theme.spacing(4),
          },
        },
      ],
    },
  },
});

export default theme;
