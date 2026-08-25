import {
  TableContainer,
  Table,
  TableHead,
  TableRow,
  TableCell,
  TableBody,
  styled,
  TableCellProps,
  TableRowProps,
} from "@mui/material";
import React, { useCallback, useLayoutEffect, useRef } from "react";

import HttpStatusIcon from "./HttpStatusIcon";

import { HttpMethod } from "lib/graphql/generated";

const baseCellStyle = {
  whiteSpace: "nowrap",
  overflow: "hidden",
  textOverflow: "ellipsis",
} as const;

const NumberTableCell = styled(TableCell)<TableCellProps>(({ theme }) => ({
  ...baseCellStyle,
  width: "50px",
  color: theme.palette.text.secondary,
  fontVariantNumeric: "tabular-nums",
}));

const MethodTableCell = styled(TableCell)<TableCellProps>(() => ({
  ...baseCellStyle,
  width: "100px",
}));

const OriginTableCell = styled(TableCell)<TableCellProps>(() => ({
  ...baseCellStyle,
  maxWidth: "100px",
}));

const PathTableCell = styled(TableCell)<TableCellProps>(() => ({
  ...baseCellStyle,
  maxWidth: "200px",
}));

const StatusTableCell = styled(TableCell)<TableCellProps>(() => ({
  ...baseCellStyle,
  width: "100px",
}));

const RequestTableRow = styled(TableRow)<TableRowProps>(() => ({
  "&:hover": {
    cursor: "pointer",
  },
}));

interface HttpRequest {
  id: string;
  url: string;
  method: HttpMethod;
  response?: HttpResponse | null;
}

interface HttpResponse {
  statusCode: number;
  statusReason: string;
  body?: string;
}

interface Props {
  requests: HttpRequest[];
  activeRowId?: string;
  actionsCell?: (id: string) => JSX.Element;
  onRowClick?: (id: string) => void;
  onContextMenu?: (e: React.MouseEvent, id: string) => void;
  oldestFirst?: boolean;
  rowNumberBase?: number;
}

interface RowProps {
  id: string;
  rowNumber: number;
  method: HttpMethod;
  url: string;
  response?: HttpResponse | null;
  isActive: boolean;
  actionsCell?: (id: string) => JSX.Element;
  onRowClick: (id: string) => void;
  onContextMenu: (e: React.MouseEvent, id: string) => void;
}

const RequestRow = React.memo(function RequestRow({
  id,
  rowNumber,
  method,
  url,
  response,
  isActive,
  actionsCell,
  onRowClick,
  onContextMenu,
}: RowProps) {
  const { origin, pathname, search, hash } = new URL(url);
  return (
    <RequestTableRow
      hover
      selected={isActive}
      onClick={() => onRowClick(id)}
      onContextMenu={(e) => {
        e.preventDefault();
        onContextMenu(e, id);
      }}
    >
      <NumberTableCell>{rowNumber}</NumberTableCell>
      <MethodTableCell>
        <code>{method}</code>
      </MethodTableCell>
      <OriginTableCell>{origin}</OriginTableCell>
      <PathTableCell>{decodeURIComponent(pathname + search + hash)}</PathTableCell>
      <StatusTableCell>
        {response && <Status code={response.statusCode} reason={response.statusReason} />}
      </StatusTableCell>
      {actionsCell && actionsCell(id)}
    </RequestTableRow>
  );
});

export default function RequestsTable(props: Props): JSX.Element {
  const { requests, activeRowId, actionsCell, onRowClick, onContextMenu, oldestFirst, rowNumberBase } = props;

  // Store latest callbacks in refs so memoized rows always call the current version
  // without needing to re-render just because the parent passed a new function reference.
  const onRowClickRef = useRef(onRowClick);
  const onContextMenuRef = useRef(onContextMenu);
  const actionsCellRef = useRef(actionsCell);
  useLayoutEffect(() => {
    onRowClickRef.current = onRowClick;
    onContextMenuRef.current = onContextMenu;
    actionsCellRef.current = actionsCell;
  });

  const stableRowClick = useCallback((id: string) => onRowClickRef.current?.(id), []);
  const stableContextMenu = useCallback((e: React.MouseEvent, id: string) => onContextMenuRef.current?.(e, id), []);
  const stableActionsCell = useCallback((id: string) => actionsCellRef.current?.(id) as JSX.Element, []);

  const rows = oldestFirst ? [...requests].reverse() : requests;

  return (
    <TableContainer sx={{ overflowX: "initial" }}>
      <Table size="small" stickyHeader>
        <TableHead>
          <TableRow>
            <TableCell>#</TableCell>
            <TableCell>Method</TableCell>
            <TableCell>Origin</TableCell>
            <TableCell>Path</TableCell>
            <TableCell>Status</TableCell>
            {actionsCell && <TableCell padding="checkbox"></TableCell>}
          </TableRow>
        </TableHead>
        <TableBody>
          {rows.map(({ id, method, url, response }, index) => {
            const base = rowNumberBase || rows.length;
            const rowNumber = oldestFirst ? index + 1 : base - index;
            return (
              <RequestRow
                key={id}
                id={id}
                rowNumber={rowNumber}
                method={method}
                url={url}
                response={response}
                isActive={id === activeRowId}
                actionsCell={actionsCell !== undefined ? stableActionsCell : undefined}
                onRowClick={stableRowClick}
                onContextMenu={stableContextMenu}
              />
            );
          })}
        </TableBody>
      </Table>
    </TableContainer>
  );
}

function Status({ code, reason }: { code: number; reason: string }): JSX.Element {
  return (
    <div>
      <HttpStatusIcon status={code} />{" "}
      <code>
        {code} {reason}
      </code>
    </div>
  );
}
