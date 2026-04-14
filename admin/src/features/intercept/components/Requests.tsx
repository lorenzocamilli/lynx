import CancelIcon from "@mui/icons-material/Cancel";
import { Alert, Box, Button, CircularProgress, Link, MenuItem, Paper, Snackbar, Typography } from "@mui/material";
import { useRouter } from "next/router";
import { useState } from "react";

import { useInterceptedRequests } from "lib/InterceptedRequestsContext";
import RequestsTable from "lib/components/RequestsTable";
import useContextMenu from "lib/components/useContextMenu";
import { useCancelRequestMutation, useCreateSenderRequestFromHttpRequestLogMutation } from "lib/graphql/generated";

function Requests(): JSX.Element {
  const interceptedRequests = useInterceptedRequests();
  const router = useRouter();
  const activeId = router.query.id as string | undefined;

  const [createSenderReqFromLog] = useCreateSenderRequestFromHttpRequestLogMutation({
    onCompleted({ createSenderRequestFromHttpRequestLog }) {
      setNewSenderReqId(createSenderRequestFromHttpRequestLog.id);
      setCopiedNotifOpen(true);
    },
  });

  const [cancelRequest] = useCancelRequestMutation();
  const [dropping, setDropping] = useState(false);

  const handleDropAll = async () => {
    if (!interceptedRequests?.length) return;
    setDropping(true);
    for (const req of interceptedRequests) {
      await cancelRequest({ variables: { id: req.id } });
    }
    setDropping(false);
    router.replace("/proxy/intercept");
  };

  const [copyToSenderId, setCopyToSenderId] = useState("");
  const [newSenderReqId, setNewSenderReqId] = useState("");
  const [copiedNotifOpen, setCopiedNotifOpen] = useState(false);
  const [Menu, handleContextMenu, handleContextMenuClose] = useContextMenu();

  const handleRowClick = (id: string) => {
    router.push(`/proxy/intercept?id=${id}`);
  };

  const handleRowContextClick = (e: React.MouseEvent, id: string) => {
    setCopyToSenderId(id);
    handleContextMenu(e);
  };

  const handleCopyToSenderClick = () => {
    createSenderReqFromLog({ variables: { id: copyToSenderId } });
    handleContextMenuClose();
  };

  const handleCloseCopiedNotif = (_: Event | React.SyntheticEvent, reason?: string) => {
    if (reason === "clickaway") return;
    setCopiedNotifOpen(false);
  };

  return (
    <Box>
      <Menu>
        <MenuItem onClick={handleCopyToSenderClick}>Copy request to Sender</MenuItem>
      </Menu>
      <Snackbar
        open={copiedNotifOpen}
        autoHideDuration={3000}
        onClose={handleCloseCopiedNotif}
        anchorOrigin={{ horizontal: "center", vertical: "bottom" }}
      >
        <Alert onClose={handleCloseCopiedNotif} severity="info">
          Request was copied. <Link href={`/sender?id=${newSenderReqId}`}>Edit in Sender.</Link>
        </Alert>
      </Snackbar>
      {interceptedRequests && interceptedRequests.length > 0 && (
        <Box sx={{ p: 1 }}>
          <Button
            size="small"
            variant="outlined"
            color="error"
            startIcon={dropping ? <CircularProgress size={16} /> : <CancelIcon />}
            onClick={handleDropAll}
            disabled={dropping}
          >
            Drop all
          </Button>
        </Box>
      )}
      {interceptedRequests && interceptedRequests.length > 0 && (
        <RequestsTable
          requests={interceptedRequests}
          onRowClick={handleRowClick}
          onContextMenu={handleRowContextClick}
          activeRowId={activeId}
        />
      )}
      <Box sx={{ mt: 2, height: "100%" }}>
        {interceptedRequests?.length === 0 && (
          <Paper variant="centered">
            <Typography>No pending intercepted requests.</Typography>
          </Paper>
        )}
      </Box>
    </Box>
  );
}

export default Requests;
