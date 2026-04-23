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
  FormControlLabel,
  Alert,
  MenuItem,
  ListSubheader,
  Select,
  FormControl,
  InputLabel,
} from '@mui/material';
import { useBackendContext } from 'src/contexts/backend-context';
import { useAlertContext } from 'src/contexts/error-context';
import ArrowPathIcon from '@heroicons/react/24/outline/ArrowPathIcon';
import PowerIcon from '@heroicons/react/24/outline/PowerIcon';
import ExclamationTriangleIcon from '@heroicons/react/24/outline/ExclamationTriangleIcon';

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

export const DevicesInfo = ({ sn, mtp, deviceOnline, onOnlineChange, onStatusRefresh }) => {
  const { httpRequest, apiPrefix } = useBackendContext();
  const { setAlert } = useAlertContext();

  const [info, setInfo] = useState(null);
  const [loading, setLoading] = useState(false);
  const [actionLoading, setActionLoading] = useState(false);
  const [isCached, setIsCached] = useState(false);
  const [cachedAt, setCachedAt] = useState(null);

  const [confirmDialog, setConfirmDialog] = useState({
    open: false,
    title: '',
    description: '',
    action: null,
  });


  // Firmware policy dropdown
  const [fwPolicy, setFwPolicy] = useState('campaign');
  const [fwPolicyFwId, setFwPolicyFwId] = useState('');
  const [fwPolicyLoading, setFwPolicyLoading] = useState(false);
  const [availableFirmware, setAvailableFirmware] = useState([]);
  const [noCampaignAlert, setNoCampaignAlert] = useState(false);
  const [upgradeLogs, setUpgradeLogs] = useState([]);

  const fetchUpgradeLogs = useCallback(async () => {
    if (!sn) return;
    try {
      const { status, result } = await httpRequest(`${apiPrefix}/device/${sn}/upgrade-logs`, 'GET');
      if (status === 200 && Array.isArray(result)) {
        setUpgradeLogs(result);
      }
    } catch {
      // ignore
    }
  }, [sn]);

  const fetchCachedInfo = useCallback(async () => {
    onStatusRefresh?.();
    if (!sn) return;
    setLoading(true);
    fetchUpgradeLogs();
    try {
      const { status, result } = await httpRequest(`${apiPrefix}/device/${sn}/cached-info`, 'GET');
      if (status === 200 && result && result.info) {
        setInfo(result.info);
        setIsCached(true);
        setCachedAt(result.updated_at);
      }
    } catch {
      // ignore
    }
    setLoading(false);
  }, [sn, fetchUpgradeLogs]);

  const fetchLiveInfo = useCallback(async () => {
    onStatusRefresh?.();
    if (!sn) return;
    setLoading(true);
    setIsCached(false);
    fetchUpgradeLogs();
    try {
      const { status, result } = await httpRequest(`${apiPrefix}/device/${sn}/${mtp}/info`, 'GET', null, null);
      if (status === 200 && result) {
        setInfo(result);
        setIsCached(false);
        setCachedAt(null);
        setLoading(false);
        if (onOnlineChange) onOnlineChange(true);
        return;
      }
    } catch {
      // ignore
    }
    // Fallback to cached if live fetch failed
    await fetchCachedInfo();
    if (onOnlineChange) onOnlineChange(false);
  }, [sn, mtp, fetchCachedInfo, fetchUpgradeLogs]);

  useEffect(() => {
    if (deviceOnline === false) {
      fetchCachedInfo();
    } else {
      fetchLiveInfo();
    }
  }, [sn]);

  // Fetch firmware policy and available firmware on mount
  useEffect(() => {
    if (!sn) return;
    const fetchPolicy = async () => {
      try {
        const { status, result } = await httpRequest(`${apiPrefix}/device/${sn}/fw-policy`, 'GET');
        if (status === 200 && result) {
          if (result.policy === 'manual' && result.manual_firmware_id) {
            setFwPolicy(result.manual_firmware_id);
            setFwPolicyFwId(result.manual_firmware_id);
          } else {
            setFwPolicy(result.policy || 'campaign');
          }
        }
      } catch {
        // ignore — default to campaign
      }
    };
    const fetchFwList = async () => {
      try {
        const { status, result } = await httpRequest(`${apiPrefix}/firmware`, 'GET');
        if (status === 200 && Array.isArray(result)) {
          setAvailableFirmware(result);
        }
      } catch {
        // ignore
      }
    };
    fetchPolicy();
    fetchFwList();
    fetchUpgradeLogs();
  }, [sn]);

  // Check if campaign exists for this device's hardware when policy is "campaign"
  useEffect(() => {
    if (fwPolicy !== 'campaign' || !sn || !info) {
      setNoCampaignAlert(false);
      return;
    }
    const checkCampaign = async () => {
      try {
        // Extract vendor/model from device info
        const flat = {};
        if (info?.req_path_results) {
          for (const r of info.req_path_results) {
            if (r.resolved_path_results) {
              for (const rr of r.resolved_path_results) {
                if (rr.result_params) Object.assign(flat, rr.result_params);
              }
            }
          }
        }
        const vendor = (flat.Manufacturer || '').toLowerCase();
        const model = (flat.ModelName || '').toLowerCase();
        const hwVersion = (flat.HardwareVersion || '').toLowerCase();

        const { status, result } = await httpRequest(`${apiPrefix}/campaigns`, 'GET');
        if (status === 200 && Array.isArray(result)) {
          const match = result.some(
            (c) =>
              c.enabled !== false &&
              (c.vendor || '').toLowerCase() === vendor &&
              (c.model || '').toLowerCase() === model &&
              (c.hw_version || '').toLowerCase() === hwVersion
          );
          setNoCampaignAlert(!match && Boolean(vendor || model));
        }
      } catch {
        // ignore
      }
    };
    checkCampaign();
  }, [fwPolicy, info]);

  const handleFwPolicyChange = async (e) => {
    const value = e.target.value;
    if (!value) return;
    setFwPolicyLoading(true);
    try {
      let policy, firmwareId;
      if (value === 'campaign') {
        policy = 'campaign';
        firmwareId = '';
      } else if (value === 'skip') {
        policy = 'skip';
        firmwareId = '';
      } else {
        policy = 'manual';
        firmwareId = value;
      }
      const body = JSON.stringify({ policy, manual_firmware_id: firmwareId });
      const { status } = await httpRequest(`${apiPrefix}/device/${sn}/fw-policy`, 'PUT', body);
      if (status === 200 || status === 204) {
        setFwPolicy(value);
        if (policy === 'manual') setFwPolicyFwId(firmwareId);
        setAlert({ severity: 'success', message: 'Firmware policy updated.' });
      }
    } catch {
      setAlert({ severity: 'error', message: 'Failed to update firmware policy.' });
    } finally {
      setFwPolicyLoading(false);
    }
  };

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

  const rows = info ? parseUspFlat(info) : [];
  const deviceVendor = rows.find(r => r.key === 'Manufacturer')?.value || '';
  const deviceModel = rows.find(r => r.key === 'ModelName')?.value || '';

  const isOffline = deviceOnline === false;

  return (
    <>
      {isOffline && (
        <Alert severity="error" variant="filled" sx={{ fontWeight: 600 }}>
          Device is Offline
        </Alert>
      )}
      {isCached && cachedAt && (
        <Alert severity="info">
          Showing cached data from {new Date(cachedAt).toLocaleString()}.
          {!isOffline && ' Click Refresh to fetch live data.'}
        </Alert>
      )}
      <Card>
        <CardHeader
          title="Device Information"
          subheader={`Serial: ${sn}`}
          action={
            <Button
              size="small"
              startIcon={<SvgIcon fontSize="small"><ArrowPathIcon /></SvgIcon>}
              onClick={isOffline ? fetchCachedInfo : fetchLiveInfo}
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
            disabled={isOffline}
            startIcon={<SvgIcon fontSize="small"><PowerIcon /></SvgIcon>}
            onClick={() =>
              openConfirm(
                'Reboot Device',
                'Are you sure you want to reboot this device? This cannot be undone.',
                `${apiPrefix}/device/${sn}/${mtp}/reboot`
              )
            }
          >
            Reboot
          </Button>
          <Button
            variant="outlined"
            color="error"
            size="small"
            disabled={isOffline}
            startIcon={<SvgIcon fontSize="small"><ExclamationTriangleIcon /></SvgIcon>}
            onClick={() =>
              openConfirm(
                'Factory Reset',
                'Are you sure you want to factory reset this device? This cannot be undone.',
                `${apiPrefix}/device/${sn}/${mtp}/factory-reset`
              )
            }
          >
            Factory Reset
          </Button>
        </CardActions>
        <Divider />
        {/* Firmware Policy Dropdown */}
        <Box sx={{ px: 2, py: 1.5 }}>
          <Stack direction="row" spacing={2} alignItems="center">
            <FormControl size="small" sx={{ minWidth: 280 }}>
              <InputLabel id="fw-policy-label">Firmware Policy</InputLabel>
              <Select
                labelId="fw-policy-label"
                id="fw-policy-select"
                value={fwPolicy}
                label="Firmware Policy"
                onChange={handleFwPolicyChange}
                disabled={fwPolicyLoading}
              >
                <MenuItem value="campaign">Use campaign firmware</MenuItem>
                <MenuItem value="skip">Do not enforce FW</MenuItem>
                <Divider />
                <ListSubheader>Manual firmware</ListSubheader>
                {availableFirmware
                  .filter((fw) => {
                    if (deviceVendor && fw.vendor && fw.vendor.toLowerCase() !== deviceVendor.toLowerCase()) return false;
                    if (deviceModel && fw.model && fw.model.toLowerCase() !== deviceModel.toLowerCase()) return false;
                    return true;
                  })
                  .map((fw) => (
                    <MenuItem key={fw.id} value={fw.id}>
                      FW {fw.name} v{fw.build_version || '?'}{fw.created_at ? ` (${new Date(fw.created_at).toLocaleDateString()})` : ''}
                    </MenuItem>
                  ))}
              </Select>
            </FormControl>
            {fwPolicyLoading && <CircularProgress size={20} />}
          </Stack>
          {noCampaignAlert && (
            <Alert severity="info" sx={{ mt: 1 }}>
              No active campaign for this hardware
            </Alert>
          )}
        </Box>
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

      {/* Upgrade History */}
      {upgradeLogs.length > 0 && (
        <Card sx={{ mt: 2 }}>
          <CardHeader title="Upgrade History" />
          <Divider />
          <TableContainer component={Paper} elevation={0}>
            <Table size="small">
              <TableHead>
                <TableRow>
                  <TableCell sx={{ fontWeight: 700, fontSize: '0.8rem' }}>Firmware</TableCell>
                  <TableCell sx={{ fontWeight: 700, fontSize: '0.8rem' }}>Status</TableCell>
                  <TableCell sx={{ fontWeight: 700, fontSize: '0.8rem' }}>Trigger</TableCell>
                  <TableCell sx={{ fontWeight: 700, fontSize: '0.8rem' }}>Date</TableCell>
                  <TableCell sx={{ fontWeight: 700, fontSize: '0.8rem' }}>Error</TableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {upgradeLogs.map((log) => (
                  <TableRow key={log.id} hover>
                    <TableCell sx={{ fontSize: '0.82rem' }}>
                      {log.firmware_name} v{log.firmware_build_ver}
                    </TableCell>
                    <TableCell>
                      <Chip
                        label={log.status}
                        size="small"
                        color={
                          log.status === 'success' ? 'success'
                          : log.status === 'failed' ? 'error'
                          : log.status === 'downloading' ? 'warning'
                          : 'info'
                        }
                        variant="outlined"
                      />
                    </TableCell>
                    <TableCell sx={{ fontSize: '0.82rem' }}>{log.trigger_type}</TableCell>
                    <TableCell sx={{ fontSize: '0.82rem' }}>
                      {new Date(log.triggered_at).toLocaleString()}
                    </TableCell>
                    <TableCell sx={{ fontSize: '0.82rem', color: 'error.main' }}>
                      {log.error || ''}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </TableContainer>
        </Card>
      )}

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
