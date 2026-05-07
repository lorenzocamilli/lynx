import { Box, Button, MenuItem, Paper, Typography } from "@mui/material";
import { useRouter } from "next/router";
import { useState } from "react";

const PAGE_SIZE = 50;

import RequestsTable from "lib/components/RequestsTable";
import useContextMenu from "lib/components/useContextMenu";
import { useDeleteSenderRequestMutation, useGetSenderRequestsQuery } from "lib/graphql/generated";

function History(): JSX.Element {
  const [limit, setLimit] = useState(PAGE_SIZE);
  const { data, loading, refetch } = useGetSenderRequestsQuery({ variables: { limit } });

  const router = useRouter();
  const activeId = router.query.id as string | undefined;

  const [contextId, setContextId] = useState("");
  const [Menu, handleContextMenu, handleContextMenuClose] = useContextMenu();
  const [deleteSenderRequest] = useDeleteSenderRequestMutation();

  const handleRowClick = (id: string) => {
    router.push(`/sender?id=${id}`);
  };

  const handleRowContextMenu = (e: React.MouseEvent, id: string) => {
    setContextId(id);
    handleContextMenu(e);
  };

  const handleDeleteClick = async () => {
    handleContextMenuClose();
    await deleteSenderRequest({ variables: { id: contextId } });
    if (activeId === contextId) {
      router.replace("/sender");
    }
    refetch();
  };

  return (
    <Box>
      <Menu>
        <MenuItem onClick={handleDeleteClick}>Delete request</MenuItem>
      </Menu>
      {!loading && data?.senderRequests && data?.senderRequests.length > 0 && (
        <>
          <RequestsTable
            requests={data.senderRequests}
            onRowClick={handleRowClick}
            onContextMenu={handleRowContextMenu}
            activeRowId={activeId}
          />
          {data.senderRequests.length >= limit && (
            <Box sx={{ display: "flex", justifyContent: "center", py: 1 }}>
              <Button size="small" onClick={() => setLimit((l) => l + PAGE_SIZE)}>
                Load more
              </Button>
            </Box>
          )}
        </>
      )}
      <Box sx={{ mt: 2, height: "100%" }}>
        {!loading && data?.senderRequests.length === 0 && (
          <Paper variant="centered">
            <Typography>No requests created yet.</Typography>
          </Paper>
        )}
      </Box>
    </Box>
  );
}

export default History;
