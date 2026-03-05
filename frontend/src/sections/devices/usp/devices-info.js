import { useState, useEffect, useCallback } from 'react';
import {
  Card,
  CardContent,
  CardHeader,
  CardActions,
  Button,
  Stack,
  Box,
  SvgIcon,
  Typography,
  CircularProgress,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableRow,
  Paper,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogContentText,
  DialogActions,
  Divider,
  Chip,
} from '@mui/material';
import { useRouter } from 'next/router';
import { useBackendContext } from 'src/contexts/backend-context';
import { useAlertContext } from 'src/contexts/error-context';
import ArrowPathIcon from '@heroicons/react/24/outline/ArrowPathIcon';
import PowerIcon from '@heroicons/react/24/outline/PowerIcon';
import ExclamationTriangleIcon from '@heroicons/react/24/outline/ExclamationTriangleIcon';
import ArrowUturnLeftIcon from '@heroicons/react/24/outline/ArrowUturnLeftIcon';

const ConfirmDialog = ({ open, onClose, onConfirm, title, description, loading }) => (
  <Dialog open={open} onClose={onClose} maxWidth="xs" fullWidth>
    <DialogTitle>{title}</DialogTitle>
    <DialogContent>
      <DialogContentText>{description}</DialogContentText>
    </DialogContent>
    <DialogActions>
      <Button onClick={onClose} disabled={loading}>Cancel</Button>
      <Button
        onClick={onConfirm}
        color="error"
        variant="contained"
        disabled={loading}
        startIcon={loading ? <CircularProgress size={16} color="inherit" /> : null}
      >
        Confirm
      </Button>
    </DialogActions>
  </Dialog>
);

// Recursively flatten a nested object into key-value pairs for display
const flattenObject = (obj, prefix = '') => {
  if (!obj || typeof obj !== 'object') return [];
  return Object.entries(obj).flatMap(([key, value]) => {
    const fullKey = prefix ? `${prefix}.${key}` : key;
    if (value !== null && typeof value === 'object' && !Array.isArray(value)) {
      return flattenObject(value, fullKey);
    }
    return [{ key: fullKey, value: Array.isArray(value) ? JSON.stringify(value) : String(value ?? '') }];
  });
};

export const DevicesInfo = ({ sn, mtp }) => {
  const router = useRouter();
  const { httpRequest } = useBackendContext();
  const { setAlert } = useAlertContext();

  const [info, setInfo] = useState(null);
  const [loading, setLoading] = useState(false);
  const [actionLoading, setActionLoading] = useState(false);

  const [confirmDialog, setConfirmDialog] = useState({
    open: false,
    title: '',
    description: '',
    action: null,
  });

  const fetchInfo = useCallback(async () => {
    if (!sn) return;
    setLoading(true);
    try {
      const { status, result } = await httpRequest(`/api/device/${sn}/${mtp}/info`, 'GET', null, null);
      if (status === 200 && result) {
        setInfo(result);
      }
    } finally {
      setLoading(false);
    }
  }, [sn, mtp, httpRequest]);

  useEffect(() => {
    fetchInfo();
  }, [fetchInfo]);

  const openConfirm = (title, description, action) => {
    setConfirmDialog({ open: true, title, description, action });
  };

  const closeConfirm = () => {
    setConfirmDialog(prev => ({ ...prev, open: false }));
  };

  const handleConfirm = async () => {
    if (!confirmDialog.action) return;
    setActionLoading(true);
    try {
      const { status } = await httpRequest(confirmDialog.action, 'PUT', null, null);
      if (status === 200 || status === 204) {
        setAlert({ severity: 'success', message: 'Action completed successfully.' });
      }
    } finally {
      setActionLoading(false);
      closeConfirm();
    }
  };

  const rows = info ? flattenObject(info) : [];

  return (
    <>
      <Card>
        <CardHeader
          title="Device Information"
          subheader={`Serial: ${sn}`}
          action={
            <Button
              size="small"
              startIcon={<SvgIcon fontSize="small"><ArrowPathIcon /></SvgIcon>}
              onClick={fetchInfo}
              disabled={loading}
            >
              Refresh
            </Button>
          }
        />
        <Divider />
        <CardActions sx={{ px: 2, py: 1.5, flexWrap: 'wrap', gap: 1 }}>
          <Button
            variant="outlined"
            color="warning"
            size="small"
            startIcon={<SvgIcon fontSize="small"><PowerIcon /></SvgIcon>}
            onClick={() =>
              openConfirm(
                'Reboot Device',
                'Are you sure you want to reboot this device? This cannot be undone.',
                `/api/device/${sn}/${mtp}/reboot`
              )
            }
          >
            Reboot
          </Button>
          <Button
            variant="outlined"
            color="error"
            size="small"
            startIcon={<SvgIcon fontSize="small"><ExclamationTriangleIcon /></SvgIcon>}
            onClick={() =>
              openConfirm(
                'Factory Reset',
                'Are you sure you want to factory reset this device? This cannot be undone.',
                `/api/device/${sn}/${mtp}/factory-reset`
              )
            }
          >
            Factory Reset
          </Button>
          <Button
            variant="outlined"
            color="info"
            size="small"
            startIcon={<SvgIcon fontSize="small"><ArrowUturnLeftIcon /></SvgIcon>}
            onClick={() =>
              openConfirm(
                'Restart Agent',
                'Are you sure you want to restart the USP agent? This cannot be undone.',
                `/api/device/${sn}/${mtp}/restart-agent`
              )
            }
          >
            Restart Agent
          </Button>
        </CardActions>
        <Divider />
        <CardContent sx={{ p: 0 }}>
          {loading ? (
            <Box display="flex" justifyContent="center" py={4}>
              <CircularProgress />
            </Box>
          ) : rows.length === 0 ? (
            <Box display="flex" justifyContent="center" py={4}>
              <Typography color="text.secondary" variant="body2">
                No device information available. Click Refresh to fetch.
              </Typography>
            </Box>
          ) : (
            <TableContainer component={Paper} elevation={0}>
              <Table size="small">
                <TableBody>
                  {rows.map(({ key, value }) => (
                    <TableRow key={key} hover>
                      <TableCell
                        sx={{
                          width: '40%',
                          fontWeight: 600,
                          color: 'text.secondary',
                          fontSize: '0.8rem',
                          wordBreak: 'break-word',
                        }}
                      >
                        {key}
                      </TableCell>
                      <TableCell sx={{ fontSize: '0.85rem', wordBreak: 'break-word' }}>
                        {value === 'true' ? (
                          <Chip label="true" size="small" color="success" variant="outlined" />
                        ) : value === 'false' ? (
                          <Chip label="false" size="small" color="default" variant="outlined" />
                        ) : (
                          value
                        )}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </TableContainer>
          )}
        </CardContent>
      </Card>

      <ConfirmDialog
        open={confirmDialog.open}
        onClose={closeConfirm}
        onConfirm={handleConfirm}
        title={confirmDialog.title}
        description={confirmDialog.description}
        loading={actionLoading}
      />
    </>
  );
};
