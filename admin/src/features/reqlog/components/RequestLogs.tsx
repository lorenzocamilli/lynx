import { Alert, Box, Button, Link, MenuItem, Snackbar } from "@mui/material";
import { useRouter } from "next/router";
import { useState } from "react";

import Actions from "./Actions";
import LogDetail from "./LogDetail";
import Search from "./Search";

import RequestsTable from "lib/components/RequestsTable";
import SplitPane from "lib/components/SplitPane";
import useContextMenu from "lib/components/useContextMenu";
import {
  useCreateSenderRequestFromHttpRequestLogMutation,
  useDeleteHttpRequestLogMutation,
  useHttpRequestLogsCountQuery,
  useHttpRequestLogsQuery,
} from "lib/graphql/generated";
import { useSSE } from "lib/useSSE";

const PAGE_SIZE = 50;

export function RequestLogs(): JSX.Element {
  const router = useRouter();
  const id = router.query.id as string | undefined;
  const [limit, setLimit] = useState(PAGE_SIZE);

  const { data, previousData, refetch } = useHttpRequestLogsQuery({
    variables: { limit, offset: 0 },
  });

  const { data: countData, refetch: refetchCount } = useHttpRequestLogsCountQuery({
    fetchPolicy: "network-only",
  });

  useSSE((type) => {
    if (type === "request_log" || type === "response_log") {
      refetch();
      refetchCount();
    }
  }, 300);

  const requests = (data ?? previousData)?.httpRequestLogs ?? [];
  const totalCount = countData?.httpRequestLogsCount ?? 0;
  const showLoadMore = requests.length >= limit;

  const [deleteHttpRequestLog] = useDeleteHttpRequestLogMutation();
  const [createSenderReqFromLog] = useCreateSenderRequestFromHttpRequestLogMutation({
    onCompleted({ createSenderRequestFromHttpRequestLog }) {
      setNewSenderReqId(createSenderRequestFromHttpRequestLog.id);
      setCopiedReqNotifOpen(true);
    },
  });

  const [copyToSenderId, setCopyToSenderId] = useState("");
  const [Menu, handleContextMenu, handleContextMenuClose] = useContextMenu();

  const handleCopyToSenderClick = () => {
    createSenderReqFromLog({ variables: { id: copyToSenderId } });
    handleContextMenuClose();
  };

  const handleDeleteClick = async () => {
    handleContextMenuClose();
    await deleteHttpRequestLog({ variables: { id: copyToSenderId } });
    if (id === copyToSenderId) {
      router.replace("/proxy/logs");
    }
    refetch();
    refetchCount();
  };

  const [newSenderReqId, setNewSenderReqId] = useState("");
  const [copiedReqNotifOpen, setCopiedReqNotifOpen] = useState(false);
  const handleCloseCopiedNotif = (_: Event | React.SyntheticEvent, reason?: string) => {
    if (reason === "clickaway") return;
    setCopiedReqNotifOpen(false);
  };

  const handleRowClick = (rowId: string) => {
    router.push(`/proxy/logs?id=${rowId}`);
  };

  const handleRowContextClick = (e: React.MouseEvent, rowId: string) => {
    setCopyToSenderId(rowId);
    handleContextMenu(e);
  };

  return (
    <Box display="flex" flexDirection="column" height="100%">
      <Box display="flex">
        <Box flex="1 auto">
          <Search />
        </Box>
        <Box pt={0.5}>
          <Actions />
        </Box>
      </Box>
      <Box sx={{ display: "flex", flex: "1 auto", position: "relative" }}>
        <SplitPane split="horizontal" size={"40%"}>
          <Box sx={{ width: "100%", height: "100%", pb: 2 }}>
            <Box sx={{ width: "100%", height: "100%", overflow: "scroll" }}>
              <Menu>
                <MenuItem onClick={handleCopyToSenderClick}>Copy request to Sender</MenuItem>
                <MenuItem onClick={handleDeleteClick}>Delete request</MenuItem>
              </Menu>
              <Snackbar
                open={copiedReqNotifOpen}
                autoHideDuration={3000}
                onClose={handleCloseCopiedNotif}
                anchorOrigin={{ horizontal: "center", vertical: "bottom" }}
              >
                <Alert onClose={handleCloseCopiedNotif} severity="info">
                  Request was copied. <Link href={`/sender?id=${newSenderReqId}`}>Edit in Sender.</Link>
                </Alert>
              </Snackbar>
              <RequestsTable
                requests={requests}
                activeRowId={id}
                onRowClick={handleRowClick}
                onContextMenu={handleRowContextClick}
                rowNumberBase={totalCount || requests.length}
              />
              {showLoadMore && (
                <Box sx={{ display: "flex", justifyContent: "center", py: 1 }}>
                  <Button size="small" onClick={() => setLimit((l) => l + PAGE_SIZE)}>
                    Load older
                  </Button>
                </Box>
              )}
            </Box>
          </Box>
          <LogDetail id={id} />
        </SplitPane>
      </Box>
    </Box>
  );
}
