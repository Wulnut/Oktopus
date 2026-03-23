import { useState, useEffect } from 'react';
import {
  Box,
  Button,
  Chip,
  CircularProgress,
  LinearProgress,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Paper,
  Typography,
  Stack,
} from '@mui/material';
import { useBackendContext } from 'src/contexts/backend-context';

const STATUS_COLORS = {
  success: 'success',
  failed: 'error',
  skipped: 'default',
  running: 'info',
  pending: 'default',
};

const formatDuration = (start, end) => {
  if (!start || !end) return '—';
  const ms = new Date(end) - new Date(start);
  if (ms < 1000) return `${ms}ms`;
  return `${(ms / 1000).toFixed(1)}s`;
};

export const MassActionDetail = ({ actionId, onCancel }) => {
  const { httpRequest } = useBackendContext();
  const [action, setAction] = useState(null);
  const [loading, setLoading] = useState(true);

  const fetchAction = async () => {
    const { status, result } = await httpRequest(`/api/mass-actions/${actionId}`, 'GET');
    if (status === 200 && result) {
      setAction(result);
    }
    setLoading(false);
  };

  useEffect(() => {
    fetchAction();
  }, [actionId]);

  // Auto-refresh while running
  useEffect(() => {
    if (!action || action.status !== 'running') return;
    const interval = setInterval(fetchAction, 5000);
    return () => clearInterval(interval);
  }, [action?.status]);

  if (loading) {
    return (
      <Box display="flex" justifyContent="center" py={3}>
        <CircularProgress size={24} />
      </Box>
    );
  }

  if (!action) return null;

  const total = action.total_devices || 0;
  const progress = action.progress || 0;
  const successPct = total > 0 ? (action.success_count / total) * 100 : 0;
  const failPct = total > 0 ? (action.failure_count / total) * 100 : 0;
  const pendingPct = 100 - successPct - failPct;

  return (
    <Box sx={{ p: 2 }}>
      <Stack direction="row" justifyContent="space-between" alignItems="center" sx={{ mb: 2 }}>
        <Stack direction="row" spacing={2} alignItems="center">
          <Typography variant="subtitle2">
            Progress: {progress}/{total}
          </Typography>
          <Chip label={`${action.success_count} success`} size="small" color="success" variant="outlined" />
          <Chip label={`${action.failure_count} failed`} size="small" color="error" variant="outlined" />
        </Stack>
        {action.status === 'running' && onCancel && (
          <Button size="small" color="error" variant="outlined" onClick={() => onCancel(actionId)}>
            Cancel
          </Button>
        )}
      </Stack>

      <Box sx={{ mb: 2 }}>
        <Box sx={{ display: 'flex', height: 8, borderRadius: 1, overflow: 'hidden' }}>
          <Box sx={{ width: `${successPct}%`, bgcolor: 'success.main' }} />
          <Box sx={{ width: `${failPct}%`, bgcolor: 'error.main' }} />
          <Box sx={{ width: `${pendingPct}%`, bgcolor: 'action.disabledBackground' }} />
        </Box>
      </Box>

      <TableContainer component={Paper} variant="outlined">
        <Table size="small">
          <TableHead>
            <TableRow>
              <TableCell sx={{ fontWeight: 700 }}>Device SN</TableCell>
              <TableCell sx={{ fontWeight: 700 }}>Status</TableCell>
              <TableCell sx={{ fontWeight: 700 }}>MTP</TableCell>
              <TableCell sx={{ fontWeight: 700 }}>Error</TableCell>
              <TableCell sx={{ fontWeight: 700 }}>Duration</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {(action.device_results || []).map((dr) => (
              <TableRow key={dr.device_sn} hover>
                <TableCell sx={{ fontSize: '0.85rem', fontWeight: 500 }}>
                  {dr.device_sn}
                </TableCell>
                <TableCell>
                  <Chip
                    label={dr.status}
                    size="small"
                    color={STATUS_COLORS[dr.status] || 'default'}
                    variant="outlined"
                  />
                </TableCell>
                <TableCell sx={{ fontSize: '0.85rem' }}>{dr.mtp || '—'}</TableCell>
                <TableCell sx={{ fontSize: '0.82rem', color: 'error.main', maxWidth: 300, wordBreak: 'break-word' }}>
                  {dr.error || '—'}
                </TableCell>
                <TableCell sx={{ fontSize: '0.85rem' }}>
                  {formatDuration(dr.started_at, dr.finished_at)}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </TableContainer>
    </Box>
  );
};
