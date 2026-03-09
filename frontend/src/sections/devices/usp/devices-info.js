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
  TableHead,
  TableRow,
  Paper,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogContentText,
  DialogActions,
  Divider,
  Chip,
  Radio,
  RadioGroup,
  FormControlLabel,
  Alert,
} from '@mui/material';
import { useBackendContext } from 'src/contexts/backend-context';
import { useAlertContext } from 'src/contexts/error-context';
import ArrowPathIcon from '@heroicons/react/24/outline/ArrowPathIcon';
import PowerIcon from '@heroicons/react/24/outline/PowerIcon';
import ExclamationTriangleIcon from '@heroicons/react/24/outline/ExclamationTriangleIcon';
import ArrowUturnLeftIcon from '@heroicons/react/24/outline/ArrowUturnLeftIcon';
import ArrowDownTrayIcon from '@heroicons/react/24/outline/ArrowDownTrayIcon';

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

const FirmwareUpdateDialog = ({ open, onClose, onConfirm, loading }) => {
  const { httpRequest } = useBackendContext();
  const [firmware, setFirmware] = useState([]);
  const [fetchLoading, setFetchLoading] = useState(false);
  const [selected, setSelected] = useState('');

  useEffect(() => {
    if (!open) return;
    setSelected('');
    setFetchLoading(true);
    httpRequest('/api/firmware', 'GET').then(({ status, result }) => {
      if (status === 200 && Array.isArray(result)) setFirmware(result);
    }).finally(() => setFetchLoading(false));
  }, [open]);

  const selectedFw = firmware.find(fw => fw.id === selected);

  return (
    <Dialog open={open} onClose={onClose} maxWidth="sm" fullWidth>
      <DialogTitle>Firmware Update</DialogTitle>
      <Divider />
      <DialogContent sx={{ p: 0 }}>
        {fetchLoading ? (
          <Box display="flex" justifyContent="center" py={4}>
            <CircularProgress />
          </Box>
        ) : firmware.length === 0 ? (
          <Box p={3}>
            <Alert severity="info">
              No firmware available. Upload a firmware image in the Firmware Management page first.
            </Alert>
          </Box>
        ) : (
          <>
            <TableContainer>
              <Table size="small">
                <TableHead>
                  <TableRow>
                    <TableCell padding="checkbox" />
                    <TableCell sx={{ fontWeight: 700, fontSize: '0.8rem' }}>Name</TableCell>
                    <TableCell sx={{ fontWeight: 700, fontSize: '0.8rem' }}>Version</TableCell>
                    <TableCell sx={{ fontWeight: 700, fontSize: '0.8rem' }}>Phase</TableCell>
                    <TableCell sx={{ fontWeight: 700, fontSize: '0.8rem' }}>Size</TableCell>
                  </TableRow>
                </TableHead>
                <TableBody>
                  {firmware.map((fw) => (
                    <TableRow
                      key={fw.id}
                      hover
                      selected={selected === fw.id}
                      onClick={() => setSelected(fw.id)}
                      sx={{ cursor: 'pointer' }}
                    >
                      <TableCell padding="checkbox">
                        <Radio
                          size="small"
                          checked={selected === fw.id}
                          onChange={() => setSelected(fw.id)}
                        />
                      </TableCell>
                      <TableCell sx={{ fontWeight: 600, fontSize: '0.82rem' }}>{fw.name}</TableCell>
                      <TableCell sx={{ fontSize: '0.82rem' }}>{fw.build_version || '—'}</TableCell>
                      <TableCell>
                        <Chip
                          label={fw.phase === 'release' ? 'Release' : 'Internal Testing'}
                          size="small"
                          color={fw.phase === 'release' ? 'success' : 'info'}
                          variant="outlined"
                        />
                      </TableCell>
                      <TableCell sx={{ fontSize: '0.82rem' }}>
                        {fw.file_size ? `${(fw.file_size / 1024).toFixed(0)} KB` : '—'}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </TableContainer>
            {selectedFw && (
              <Box px={2} py={1.5} sx={{ bgcolor: 'action.hover' }}>
                <Typography variant="caption" color="text.secondary">
                  URL: {selectedFw.download_url || '—'}
                </Typography>
              </Box>
            )}
          </>
        )}
      </DialogContent>
      <Divider />
      <DialogActions>
        <Button onClick={onClose} disabled={loading}>Cancel</Button>
        <Button
          onClick={() => onConfirm(selectedFw?.download_url)}
          variant="contained"
          color="primary"
          disabled={loading || !selected || !selectedFw?.download_url}
          startIcon={loading ? <CircularProgress size={16} color="inherit" /> : <SvgIcon fontSize="small"><ArrowDownTrayIcon /></SvgIcon>}
        >
          Deploy
        </Button>
      </DialogActions>
    </Dialog>
  );
};

// Extract all result_params from USP GetResp format into flat key-value pairs
const parseUspFlat = (data) => {
  if (!data?.req_path_results) return [];
  const flat = {};
  for (const r of data.req_path_results) {
    if (r.resolved_path_results) {
      for (const rr of r.resolved_path_results) {
        if (rr.result_params) Object.assign(flat, rr.result_params);
      }
    }
  }
  return Object.entries(flat).map(([key, value]) => ({ key, value: String(value ?? '') }));
};

export const DevicesInfo = ({ sn, mtp }) => {
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

  const [fwDialogOpen, setFwDialogOpen] = useState(false);
  const [fwLoading, setFwLoading] = useState(false);

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
  }, [sn, mtp]);

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

  const handleFwDeploy = async (downloadUrl) => {
    if (!downloadUrl) return;
    setFwLoading(true);
    try {
      const body = JSON.stringify({ Url: downloadUrl });
      const headers = new Headers();
      headers.append('Content-Type', 'application/json');
      headers.append('Authorization', localStorage.getItem('token'));
      const { status } = await httpRequest(`/api/device/${sn}/${mtp}/fw_update`, 'PUT', body, headers);
      if (status === 200 || status === 204) {
        setAlert({ severity: 'success', message: 'Firmware update initiated. The device will download and install the image.' });
        setFwDialogOpen(false);
      } else {
        setAlert({ severity: 'error', message: 'Firmware update failed. Check the device logs.' });
      }
    } finally {
      setFwLoading(false);
    }
  };

  const rows = info ? parseUspFlat(info) : [];

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
            color="primary"
            size="small"
            startIcon={<SvgIcon fontSize="small"><ArrowDownTrayIcon /></SvgIcon>}
            onClick={() => setFwDialogOpen(true)}
          >
            Firmware Update
          </Button>
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

      <FirmwareUpdateDialog
        open={fwDialogOpen}
        onClose={() => setFwDialogOpen(false)}
        onConfirm={handleFwDeploy}
        loading={fwLoading}
      />

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
