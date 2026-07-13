import { Box, BoxProps } from "@mui/material";
import React from "react";

interface TabPanelProps extends Omit<BoxProps, "hidden"> {
  // The tab this panel belongs to.
  value: string;
  // The currently selected tab.
  selected: string;
}

// TabPanel replaces @mui/lab's TabPanel: it renders its children only when its
// value matches the selected tab, matching lab's default unmount-on-inactive
// behaviour (so Monaco editors and scroll positions reset between tabs exactly
// as before). Pairs with @mui/material's Tabs/Tab, dropping the @mui/lab
// dependency (and its deprecated @mui/base transitive).
export default function TabPanel(props: TabPanelProps): JSX.Element | null {
  const { value, selected, children, ...boxProps } = props;

  if (value !== selected) {
    return null;
  }

  return (
    <Box role="tabpanel" {...boxProps}>
      {children}
    </Box>
  );
}
