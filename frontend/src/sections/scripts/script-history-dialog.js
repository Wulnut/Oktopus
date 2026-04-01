import { useCallback, useEffect, useState } from 'react';
import {
  Box,
  Button,
  Chip,
  Collapse,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  IconButton,
  LinearProgress,
  Stack,
  SvgIcon,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Typography,
} from '@mui/material';
import ChevronDownIcon from '@heroicons/react/24/solid/ChevronDownIcon';
import ChevronRightIcon from '@heroicons/react/24/solid/ChevronRightIcon';
import { useBackendContext } from 'src/contexts/backend-context';

const statusColor = (status) => {
  switch (status) {
    case 'completed': return 'success';
    case 'failed': return 'error';
    case 'running': return 'warning';
    default: return 'default';
  }
};

const ExecutionRow = ({ execution }) => {
  const [expanded, setExpanded] = useState(false);
  const duration = execution.started_at && execution.finished_at
    ? ((new Date(execution.finished_at) - new Date(execution.started_at)) / 1000).toFixed(1)
    : '-';

  return (
    <>
      <TableRow hover sx={{ cursor: 'pointer' }} onClick={() => setExpanded(!expanded)}>
        <TableCell sx={{ width: 40 }}>
          <IconButton size="small">
            <SvgIcon fontSize="small">
              {expanded ? <ChevronDownIcon /> : <ChevronRightIcon />}
            </SvgIcon>
          </IconButton>
        </TableCell>
        <TableCell>
          <Typography variant="body2">{execution.device_sn}</Typography>
        </TableCell>
        <TableCell>
          <Chip label={execution.status} color={statusColor(execution.status)} size="small" />
        </TableCell>
        <TableCell>
          <Typography variant="body2">{execution.mtp}</Typography>
        </TableCell>
        <TableCell>
          <Typography variant="body2">{duration}s</Typography>
        </TableCell>
        <TableCell>
          <Typography variant="body2">
            {execution.created_at ? new Date(execution.created_at).toLocaleString() : '-'}
          </Typography>
        </TableCell>
      </TableRow>
      <TableRow>
        <TableCell colSpan={6} sx={{ p: 0, borderBottom: expanded ? undefined : 'none' }}>
          <Collapse in={expanded} unmountOnExit>
            <Box sx={{ p: 2, bgcolor: 'grey.50' }}>
              {/* Variables used */}
              {execution.variables && Object.keys(execution.variables).length > 0 && (
                <Box sx={{ mb: 2 }}>
                  <Typography variant="caption" color="text.secondary">Variables</Typography>
                  <Stack direction="row" spacing={1} flexWrap="wrap" useFlexGap sx={{ mt: 0.5 }}>
                    {Object.entries(execution.variables).map(([k, v]) => (
                      <Chip key={k} label={`${k} = ${v}`} size="small" variant="outlined" />
                    ))}
                  </Stack>
                </Box>
              )}

              {/* Step results */}
              <Typography variant="caption" color="text.secondary">Steps</Typography>
              <Table size="small" sx={{ mt: 0.5 }}>
                <TableHead>
                  <TableRow>
                    <TableCell>#</TableCell>
                    <TableCell>Name</TableCell>
                    <TableCell>Status</TableCell>
                    <TableCell>Details</TableCell>
                  </TableRow>
                </TableHead>
                <TableBody>
                  {(execution.step_results || []).map((sr, i) => (
                    <TableRow key={i}>
                      <TableCell>{i + 1}</TableCell>
                      <TableCell>{sr.step_name || sr.step_id}</TableCell>
                      <TableCell>
                        <Chip
                          label={sr.status}
                          size="small"
                          color={sr.status === 'success' ? 'success' : 'error'}
                          variant="outlined"
                        />
                      </TableCell>
                      <TableCell>
                        {sr.error && (
                          <Typography variant="caption" color="error">{sr.error}</Typography>
                        )}
                        {sr.condition_result !== undefined && sr.condition_result !== null && (
                          <Typography variant="caption">
                            Condition: {sr.condition_result ? 'true' : 'false'}
                            {sr.jumped_to ? ` → ${sr.jumped_to}` : ''}
                          </Typography>
                        )}
                        {sr.response && (
                          <Box
                            component="pre"
                            sx={{
                              mt: 0.5,
                              p: 0.5,
                              fontSize: 10,
                              maxHeight: 100,
                              overflow: 'auto',
                              whiteSpace: 'pre-wrap',
                              wordBreak: 'break-all',
                            }}
                          >
                            {JSON.stringify(sr.response, null, 2)}
                          </Box>
                        )}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </Box>
          </Collapse>
        </TableCell>
      </TableRow>
    </>
  );
};

export const ScriptHistoryDialog = ({ open, onClose, script }) => {
  const { httpRequest, apiPrefix } = useBackendContext();
  const [executions, setExecutions] = useState([]);
  const [loading, setLoading] = useState(false);

  const fetchHistory = useCallback(async () => {
    if (!script?.id) return;
    setLoading(true);
    try {
      const { status, result } = await httpRequest(`${apiPrefix}/scripts/${script.id}/executions`, 'GET');
      if (status === 200 && Array.isArray(result)) {
        setExecutions(result);
      }
    } finally {
      setLoading(false);
    }
  }, [script?.id]);

  useEffect(() => {
    if (open) {
      fetchHistory();
    }
  }, [open, fetchHistory]);

  return (
    <Dialog open={open} onClose={onClose} maxWidth="lg" fullWidth scroll="paper">
      <DialogTitle>Execution History: {script?.name}</DialogTitle>
      <DialogContent dividers>
        {loading && <LinearProgress />}
        {!loading && executions.length === 0 && (
          <Typography color="text.secondary" sx={{ textAlign: 'center', py: 4 }}>
            No executions yet.
          </Typography>
        )}
        {executions.length > 0 && (
          <TableContainer>
            <Table>
              <TableHead>
                <TableRow>
                  <TableCell />
                  <TableCell>Device</TableCell>
                  <TableCell>Status</TableCell>
                  <TableCell>MTP</TableCell>
                  <TableCell>Duration</TableCell>
                  <TableCell>Date</TableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {executions.map((exec) => (
                  <ExecutionRow key={exec.id} execution={exec} />
                ))}
              </TableBody>
            </Table>
          </TableContainer>
        )}
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>Close</Button>
        <Button onClick={fetchHistory} disabled={loading}>Refresh</Button>
      </DialogActions>
    </Dialog>
  );
};
