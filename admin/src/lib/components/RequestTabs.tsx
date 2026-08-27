import { Box, Tab, Tabs } from "@mui/material";
import React, { useState } from "react";

import { KeyValuePairTable, KeyValuePair, KeyValuePairTableProps } from "./KeyValuePair";
import TabPanel from "./TabPanel";

import Editor from "lib/components/Editor";

enum TabValue {
  QueryParams = "queryParams",
  Headers = "headers",
  Body = "body",
}

interface RequestTabsProps {
  queryParams: KeyValuePair[];
  headers: KeyValuePair[];
  onQueryParamChange?: KeyValuePairTableProps["onChange"];
  onQueryParamDelete?: KeyValuePairTableProps["onDelete"];
  onHeaderChange?: KeyValuePairTableProps["onChange"];
  onHeaderDelete?: KeyValuePairTableProps["onDelete"];
  body?: string | null;
  onBodyChange?: (value: string) => void;
  hideQueryParams?: boolean;
}

function RequestTabs(props: RequestTabsProps): JSX.Element {
  const {
    queryParams,
    onQueryParamChange,
    onQueryParamDelete,
    headers,
    onHeaderChange,
    onHeaderDelete,
    body,
    onBodyChange,
    hideQueryParams,
  } = props;
  const [tabValue, setTabValue] = useState(hideQueryParams ? TabValue.Headers : TabValue.QueryParams);

  const tabSx = {
    textTransform: "none",
  };

  const queryParamsLength = onQueryParamChange ? queryParams.length - 1 : queryParams.length;
  const headersLength = onHeaderChange ? headers.length - 1 : headers.length;

  return (
    <Box sx={{ display: "flex", flexDirection: "column", height: "100%" }}>
      <Box sx={{ borderBottom: 1, borderColor: "divider", mb: 1 }}>
        <Tabs value={tabValue} onChange={(_, value) => setTabValue(value)}>
          {!hideQueryParams && (
            <Tab
              value={TabValue.QueryParams}
              label={"Query Params" + (queryParamsLength ? ` (${queryParamsLength})` : "")}
              sx={tabSx}
            />
          )}
          <Tab value={TabValue.Headers} label={"Headers" + (headersLength ? ` (${headersLength})` : "")} sx={tabSx} />
          <Tab
            value={TabValue.Body}
            label={"Body" + (body?.length ? ` (${body.length} byte` + (body.length > 1 ? "s" : "") + ")" : "")}
            sx={tabSx}
          />
        </Tabs>
      </Box>
      <Box flex="1 auto" overflow="scroll" height="100%">
        {!hideQueryParams && (
          <TabPanel value={TabValue.QueryParams} selected={tabValue} sx={{ p: 0, height: "100%" }}>
            <Box>
              <KeyValuePairTable items={queryParams} onChange={onQueryParamChange} onDelete={onQueryParamDelete} />
            </Box>
          </TabPanel>
        )}
        <TabPanel value={TabValue.Headers} selected={tabValue} sx={{ p: 0, height: "100%" }}>
          <Box>
            <KeyValuePairTable items={headers} onChange={onHeaderChange} onDelete={onHeaderDelete} />
          </Box>
        </TabPanel>
        <TabPanel value={TabValue.Body} selected={tabValue} sx={{ p: 0, height: "100%" }}>
          <Editor
            content={body || ""}
            onChange={(value) => {
              onBodyChange && onBodyChange(value || "");
            }}
            monacoOptions={{ readOnly: onBodyChange === undefined }}
            contentType={headers.find(({ key }) => key.toLowerCase() === "content-type")?.value}
          />
        </TabPanel>
      </Box>
    </Box>
  );
}

export default RequestTabs;
