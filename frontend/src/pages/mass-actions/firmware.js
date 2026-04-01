import React, { useCallback, useEffect, useState } from 'react';
import Head from 'next/head';
import {
  Autocomplete,
  Box,
  Button,
  Card,
  CardContent,
  CardHeader,
  Chip,
  CircularProgress,
  Collapse,
  Container,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  Divider,
  FormControlLabel,
  IconButton,
  InputAdornment,
  MenuItem,
  Paper,
  Stack,
  SvgIcon,
  Switch,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  TextField,
  Tooltip,
  Typography,
} from '@mui/material';
import InformationCircleIcon from '@heroicons/react/24/outline/InformationCircleIcon';
import PlusIcon from '@heroicons/react/24/solid/PlusIcon';
import PencilIcon from '@heroicons/react/24/outline/PencilIcon';
import TrashIcon from '@heroicons/react/24/outline/TrashIcon';
import ChevronDownIcon from '@heroicons/react/24/outline/ChevronDownIcon';
import ChevronUpIcon from '@heroicons/react/24/outline/ChevronUpIcon';
import { Layout as DashboardLayout } from 'src/layouts/dashboard/layout';
import { useBackendContext } from 'src/contexts/backend-context';
import { useAlertContext } from 'src/contexts/error-context';

const LOG_STATUS_COLORS = {
  pending: 'warning',
  downloading: 'info',
  success: 'success',
  failed: 'error',
};

const PAGE_SIZE = 20;

// ─── Campaign Form Dialog ───────────────────────────────────────────────────

// Sanitize input: trim whitespace, remove control characters
const sanitize = (str) => str.replace(/[\x00-\x1f\x7f]/g, '').trim();

const CampaignDialog = ({ open, onClose, onSave, campaign, firmware, campaigns }) => {
  const isEdit = Boolean(campaign);

  const [form, setForm] = useState({
    vendor: '',
    model: '',
    hw_version: '',
    firmware_id: '',
    concurrency: 10,
    time_window_enabled: false,
    time_window_start: '00:00',
    time_window_end: '06:00',
    enabled: true,
  });
  const [saving, setSaving] = useState(false);
  const [submitted, setSubmitted] = useState(false);

  useEffect(() => {
    if (!open) return;
    setSubmitted(false);
    if (campaign) {
      const hasWindow = Boolean(campaign.time_window_start || campaign.time_window_end);
      setForm({
        vendor: campaign.vendor || '',
        model: campaign.model || '',
        hw_version: campaign.hw_version || '',
        firmware_id: campaign.firmware_id || '',
        concurrency: campaign.concurrency || 10,
        time_window_enabled: hasWindow,
        time_window_start: campaign.time_window_start || '00:00',
        time_window_end: campaign.time_window_end || '06:00',
        enabled: campaign.enabled !== false,
      });
    } else {
      setForm({
        vendor: '',
        model: '',
        hw_version: '',
        firmware_id: '',
        concurrency: 10,
        time_window_enabled: false,
        time_window_start: '00:00',
        time_window_end: '06:00',
        enabled: true,
      });
    }
  }, [open, campaign]);

  // Combine firmware + campaigns as source for cascading options
  const allEntries = [
    ...firmware.map((fw) => ({ vendor: fw.vendor, model: fw.model, hw_version: fw.hw_version })),
    ...campaigns.map((c) => ({ vendor: c.vendor, model: c.model, hw_version: c.hw_version })),
  ];

  // Vendor: all unique vendors
  const vendorOptions = [...new Set(allEntries.map((e) => e.vendor).filter(Boolean))].sort();

  // Model: filtered by selected vendor
  const modelOptions = [...new Set(
    allEntries
      .filter((e) => form.vendor && e.vendor && e.vendor.toLowerCase() === form.vendor.trim().toLowerCase())
      .map((e) => e.model)
      .filter(Boolean)
  )].sort();

  // HW Version: filtered by selected vendor + model
  const hwVersionOptions = [...new Set(
    allEntries
      .filter((e) =>
        form.vendor && e.vendor && e.vendor.toLowerCase() === form.vendor.trim().toLowerCase() &&
        form.model && e.model && e.model.toLowerCase() === form.model.trim().toLowerCase()
      )
      .map((e) => e.hw_version)
      .filter(Boolean)
  )].sort();

  // Filter firmware by vendor+model+hw_version if they are set
  const filteredFw = firmware.filter((fw) => {
    if (form.vendor.trim()) {
      if (!fw.vendor || fw.vendor.toLowerCase() !== form.vendor.trim().toLowerCase()) return false;
    }
    if (form.model.trim()) {
      if (!fw.model || fw.model.toLowerCase() !== form.model.trim().toLowerCase()) return false;
    }
    if (form.hw_version.trim()) {
      if (!fw.hw_version || fw.hw_version.toLowerCase() !== form.hw_version.trim().toLowerCase()) return false;
    }
    return true;
  });

  const handleChange = (field) => (e) => {
    setForm((prev) => ({ ...prev, [field]: e.target.value }));
  };

  const canSubmit =
    form.vendor.trim() !== '' &&
    form.model.trim() !== '' &&
    form.hw_version.trim() !== '' &&
    form.firmware_id !== '';

  const handleSubmit = async () => {
    setSubmitted(true);
    if (!canSubmit) return;
    setSaving(true);
    const body = {
      vendor: sanitize(form.vendor),
      model: sanitize(form.model),
      hw_version: sanitize(form.hw_version),
      firmware_id: form.firmware_id,
      concurrency: Number(form.concurrency) || 10,
      time_window_start: form.time_window_enabled ? form.time_window_start : '',
      time_window_end: form.time_window_enabled ? form.time_window_end : '',
      enabled: form.enabled,
    };
    try {
      await onSave(body);
    } finally {
      setSaving(false);
    }
  };

  return (
    <Dialog open={open} onClose={onClose} maxWidth="sm" fullWidth>
      <DialogTitle>{isEdit ? 'Edit Campaign' : 'Create Campaign'}</DialogTitle>
      <Divider />
      <DialogContent>
        <Stack spacing={2.5} sx={{ mt: 1 }}>
          <Autocomplete
            freeSolo
            options={vendorOptions}
            value={form.vendor}
            onInputChange={(_, value) =>
              setForm((prev) => ({
                ...prev,
                vendor: value,
                ...(value !== prev.vendor ? { model: '', hw_version: '', firmware_id: '' } : {}),
              }))
            }
            disabled={isEdit}
            renderInput={(params) => (
              <TextField
                {...params}
                label="Vendor *"
                id="campaign-vendor"
                autoComplete="off"
                size="small"
                error={submitted && form.vendor.trim() === ''}
              />
            )}
          />
          <Autocomplete
            freeSolo
            options={modelOptions}
            value={form.model}
            onInputChange={(_, value) =>
              setForm((prev) => ({
                ...prev,
                model: value,
                ...(value !== prev.model ? { hw_version: '', firmware_id: '' } : {}),
              }))
            }
            disabled={isEdit || !form.vendor.trim()}
            renderInput={(params) => (
              <TextField
                {...params}
                label="Model *"
                id="campaign-model"
                autoComplete="off"
                size="small"
                error={submitted && form.model.trim() === ''}
              />
            )}
          />
          <Autocomplete
            freeSolo
            options={hwVersionOptions}
            value={form.hw_version}
            onInputChange={(_, value) =>
              setForm((prev) => ({
                ...prev,
                hw_version: value,
                ...(value !== prev.hw_version ? { firmware_id: '' } : {}),
              }))
            }
            disabled={isEdit || !form.model.trim()}
            renderInput={(params) => (
              <TextField
                {...params}
                label="HW Version *"
                id="campaign-hw-version"
                autoComplete="off"
                size="small"
                error={submitted && form.hw_version.trim() === ''}
              />
            )}
          />
          <TextField
            label="Firmware *"
            id="campaign-firmware"
            autoComplete="off"
            value={form.firmware_id}
            onChange={handleChange('firmware_id')}
            select
            fullWidth
            size="small"
            error={submitted && form.firmware_id === ''}
          >
            {filteredFw.length === 0 && (
              <MenuItem value="" disabled>
                No firmware available
              </MenuItem>
            )}
            {filteredFw.map((fw) => (
              <MenuItem key={fw.id} value={fw.id}>
                {fw.name} v{fw.build_version || '?'}
              </MenuItem>
            ))}
          </TextField>
          <TextField
            label="Concurrency"
            id="campaign-concurrency"
            autoComplete="off"
            type="number"
            value={form.concurrency}
            onChange={(e) => {
              const val = Math.max(1, Math.min(50, Number(e.target.value) || 1));
              setForm((prev) => ({ ...prev, concurrency: val }));
            }}
            inputProps={{ min: 1, max: 50 }}
            InputProps={{
              endAdornment: (
                <InputAdornment position="end">
                  <Tooltip title="The amount of devices that can be upgraded at the same time">
                    <SvgIcon fontSize="small" sx={{ color: 'action.active', cursor: 'help' }}>
                      <InformationCircleIcon />
                    </SvgIcon>
                  </Tooltip>
                </InputAdornment>
              ),
            }}
            fullWidth
            size="small"
          />
          <FormControlLabel
            control={
              <Switch
                checked={form.time_window_enabled}
                onChange={(e) =>
                  setForm((prev) => ({ ...prev, time_window_enabled: e.target.checked }))
                }
              />
            }
            label="Time Window"
          />
          {form.time_window_enabled && (
            <Stack direction="row" spacing={2}>
              <TextField
                label="Start (UTC)"
                id="campaign-tw-start"
                autoComplete="off"
                type="time"
                value={form.time_window_start}
                onChange={handleChange('time_window_start')}
                size="small"
                fullWidth
                InputLabelProps={{ shrink: true }}
              />
              <TextField
                label="End (UTC)"
                id="campaign-tw-end"
                autoComplete="off"
                type="time"
                value={form.time_window_end}
                onChange={handleChange('time_window_end')}
                size="small"
                fullWidth
                InputLabelProps={{ shrink: true }}
              />
            </Stack>
          )}
        </Stack>
      </DialogContent>
      <Divider />
      <DialogActions>
        <Button onClick={onClose} disabled={saving}>
          Cancel
        </Button>
        <Button
          onClick={handleSubmit}
          variant="contained"
          disabled={saving}
          startIcon={saving ? <CircularProgress size={16} color="inherit" /> : null}
        >
          {isEdit ? 'Save' : 'Create'}
        </Button>
      </DialogActions>
    </Dialog>
  );
};

// ─── Upgrade Log (expanded row) ─────────────────────────────────────────────

const CampaignLogs = ({ campaignId }) => {
  const { httpRequest, apiPrefix } = useBackendContext();
  const [logs, setLogs] = useState([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(0);
  const [loading, setLoading] = useState(true);

  const fetchLogs = useCallback(async () => {
    setLoading(true);
    try {
      const { status, result } = await httpRequest(
        `${apiPrefix}/campaigns/${campaignId}/logs?page=${page}&page_size=${PAGE_SIZE}`,
        'GET'
      );
      if (status === 200 && result) {
        setLogs(Array.isArray(result.logs) ? result.logs : []);
        setTotal(result.total || 0);
      }
    } finally {
      setLoading(false);
    }
  }, [campaignId, page]);

  useEffect(() => {
    fetchLogs();
  }, [fetchLogs]);

  const totalPages = Math.ceil(total / PAGE_SIZE);

  if (loading) {
    return (
      <Box display="flex" justifyContent="center" py={3}>
        <CircularProgress size={24} />
      </Box>
    );
  }

  return (
    <Box sx={{ p: 2 }}>
      {/* Summary bar */}
      <Stack direction="row" spacing={2} alignItems="center" sx={{ mb: 2 }}>
        <Typography variant="subtitle2">Upgrade Logs</Typography>
        <Chip label={`Total: ${total}`} size="small" variant="outlined" />
      </Stack>

      {logs.length === 0 ? (
        <Typography color="text.secondary" variant="body2">
          No upgrade logs for this campaign yet.
        </Typography>
      ) : (
        <>
          <TableContainer component={Paper} variant="outlined">
            <Table size="small">
              <TableHead>
                <TableRow>
                  <TableCell sx={{ fontWeight: 700 }}>Device SN</TableCell>
                  <TableCell sx={{ fontWeight: 700 }}>Previous Version</TableCell>
                  <TableCell sx={{ fontWeight: 700 }}>Target Version</TableCell>
                  <TableCell sx={{ fontWeight: 700 }}>Trigger</TableCell>
                  <TableCell sx={{ fontWeight: 700 }}>Status</TableCell>
                  <TableCell sx={{ fontWeight: 700 }}>Error</TableCell>
                  <TableCell sx={{ fontWeight: 700 }}>Triggered At</TableCell>
                  <TableCell sx={{ fontWeight: 700 }}>Completed At</TableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {logs.map((log, idx) => (
                  <TableRow key={log.id || idx} hover>
                    <TableCell sx={{ fontSize: '0.85rem', fontWeight: 500 }}>
                      {log.device_sn || '—'}
                    </TableCell>
                    <TableCell sx={{ fontSize: '0.85rem' }}>{log.previous_version || '—'}</TableCell>
                    <TableCell sx={{ fontSize: '0.85rem' }}>{log.firmware_build_ver || '—'}</TableCell>
                    <TableCell sx={{ fontSize: '0.85rem' }}>{log.trigger_type || '—'}</TableCell>
                    <TableCell>
                      <Chip
                        label={log.status || 'unknown'}
                        size="small"
                        color={LOG_STATUS_COLORS[log.status] || 'default'}
                        variant="outlined"
                      />
                    </TableCell>
                    <TableCell
                      sx={{
                        fontSize: '0.82rem',
                        color: 'error.main',
                        maxWidth: 250,
                        wordBreak: 'break-word',
                      }}
                    >
                      {log.error || '—'}
                    </TableCell>
                    <TableCell sx={{ fontSize: '0.82rem' }}>
                      {log.triggered_at ? new Date(log.triggered_at).toLocaleString() : '—'}
                    </TableCell>
                    <TableCell sx={{ fontSize: '0.82rem' }}>
                      {log.completed_at ? new Date(log.completed_at).toLocaleString() : '—'}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </TableContainer>

          {/* Pagination */}
          {totalPages > 1 && (
            <Stack direction="row" spacing={1} justifyContent="flex-end" alignItems="center" sx={{ mt: 1 }}>
              <Button size="small" disabled={page === 0} onClick={() => setPage((p) => p - 1)}>
                Previous
              </Button>
              <Typography variant="body2" color="text.secondary">
                Page {page + 1} of {totalPages}
              </Typography>
              <Button size="small" disabled={page >= totalPages - 1} onClick={() => setPage((p) => p + 1)}>
                Next
              </Button>
            </Stack>
          )}
        </>
      )}
    </Box>
  );
};

// ─── Main Page ──────────────────────────────────────────────────────────────

const Page = () => {
  const { httpRequest, apiPrefix } = useBackendContext();
  const { setAlert } = useAlertContext();

  const [campaigns, setCampaigns] = useState([]);
  const [loading, setLoading] = useState(true);
  const [firmware, setFirmware] = useState([]);
  const [expandedId, setExpandedId] = useState(null);

  // Dialog state
  const [dialogOpen, setDialogOpen] = useState(false);
  const [editingCampaign, setEditingCampaign] = useState(null);

  // Delete confirmation
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [campaignToDelete, setCampaignToDelete] = useState(null);
  const [deleting, setDeleting] = useState(false);

  const fetchCampaigns = useCallback(async () => {
    setLoading(true);
    try {
      const { status, result } = await httpRequest(`${apiPrefix}/campaigns`, 'GET');
      if (status === 200 && Array.isArray(result)) {
        setCampaigns(result);
      }
    } catch {
      setAlert({ severity: 'error', message: 'Failed to load campaigns.' });
    } finally {
      setLoading(false);
    }
  }, []);

  const fetchFirmware = useCallback(async () => {
    try {
      const { status, result } = await httpRequest(`${apiPrefix}/firmware`, 'GET');
      if (status === 200 && Array.isArray(result)) {
        setFirmware(result);
      }
    } catch {
      // ignore
    }
  }, []);

  useEffect(() => {
    fetchCampaigns();
    fetchFirmware();
  }, [fetchCampaigns, fetchFirmware]);

  // Resolve firmware name+version by id
  const getFirmwareLabel = (fwId) => {
    const fw = firmware.find((f) => f.id === fwId);
    if (!fw) return fwId || '—';
    return `${fw.name} v${fw.build_version || '?'}`;
  };

  const formatTimeWindow = (start, end) => {
    if (!start && !end) return 'Anytime';
    return `${start || '?'} - ${end || '?'}`;
  };

  // ─── Handlers ───────────────────────────────────────────────────────────

  const handleCreate = () => {
    setEditingCampaign(null);
    setDialogOpen(true);
  };

  const handleEdit = (campaign) => {
    setEditingCampaign(campaign);
    setDialogOpen(true);
  };

  const handleDialogClose = () => {
    setDialogOpen(false);
    setEditingCampaign(null);
  };

  const handleSave = async (body) => {
    if (editingCampaign) {
      const { status } = await httpRequest(
        `${apiPrefix}/campaigns/${editingCampaign.id}`,
        'PUT',
        JSON.stringify(body)
      );
      if (status === 200 || status === 204) {
        setAlert({ severity: 'success', message: 'Campaign updated.' });
        handleDialogClose();
        fetchCampaigns();
      } else {
        setAlert({ severity: 'error', message: 'Failed to update campaign.' });
      }
    } else {
      const { status } = await httpRequest(`${apiPrefix}/campaigns`, 'POST', JSON.stringify(body));
      if (status === 200 || status === 201) {
        setAlert({ severity: 'success', message: 'Campaign created.' });
        handleDialogClose();
        fetchCampaigns();
      } else if (status === 409) {
        setAlert({
          severity: 'error',
          message: 'A campaign for this vendor/model/hw_version already exists.',
        });
      } else {
        setAlert({ severity: 'error', message: 'Failed to create campaign.' });
      }
    }
  };

  const handleDeleteRequest = (campaign) => {
    setCampaignToDelete(campaign);
    setDeleteDialogOpen(true);
  };

  const handleDeleteConfirm = async () => {
    if (!campaignToDelete) return;
    setDeleting(true);
    try {
      const { status } = await httpRequest(`${apiPrefix}/campaigns/${campaignToDelete.id}`, 'DELETE');
      if (status === 200 || status === 204) {
        setAlert({ severity: 'success', message: 'Campaign deleted.' });
        setCampaigns((prev) => prev.filter((c) => c.id !== campaignToDelete.id));
      }
    } finally {
      setDeleting(false);
      setDeleteDialogOpen(false);
      setCampaignToDelete(null);
    }
  };

  const handleEnabledToggle = async (campaign) => {
    const body = JSON.stringify({
      firmware_id: campaign.firmware_id,
      concurrency: campaign.concurrency,
      time_window_start: campaign.time_window_start || '',
      time_window_end: campaign.time_window_end || '',
      enabled: !campaign.enabled,
    });
    const { status } = await httpRequest(`${apiPrefix}/campaigns/${campaign.id}`, 'PUT', body);
    if (status === 200 || status === 204) {
      setCampaigns((prev) =>
        prev.map((c) => (c.id === campaign.id ? { ...c, enabled: !campaign.enabled } : c))
      );
    } else {
      setAlert({ severity: 'error', message: 'Failed to toggle campaign.' });
    }
  };

  const handleRowClick = (id) => {
    setExpandedId(expandedId === id ? null : id);
  };

  return (
    <>
      <Head>
        <title>Firmware Campaigns | Oktopus</title>
      </Head>
      <Box component="main" sx={{ flexGrow: 1, py: 8 }}>
        <Container maxWidth="xl">
          <Stack spacing={3}>
            <Typography variant="h4">Firmware Campaigns</Typography>

            {/* Card 1 — Campaigns Table */}
            <Card>
              <CardHeader
                title="Campaigns"
                action={
                  <Button
                    variant="contained"
                    size="small"
                    startIcon={<SvgIcon fontSize="small"><PlusIcon /></SvgIcon>}
                    onClick={handleCreate}
                  >
                    Create Campaign
                  </Button>
                }
              />
              <Divider />
              <CardContent sx={{ p: 0 }}>
                {loading ? (
                  <Box display="flex" justifyContent="center" py={4}>
                    <CircularProgress />
                  </Box>
                ) : campaigns.length === 0 ? (
                  <Box display="flex" justifyContent="center" py={4}>
                    <Typography color="text.secondary" variant="body2">
                      No campaigns yet. Create one to get started.
                    </Typography>
                  </Box>
                ) : (
                  <Table size="small">
                    <TableHead>
                      <TableRow>
                        <TableCell sx={{ width: 40 }} />
                        <TableCell sx={{ fontWeight: 700 }}>Vendor</TableCell>
                        <TableCell sx={{ fontWeight: 700 }}>Model</TableCell>
                        <TableCell sx={{ fontWeight: 700 }}>HW Version</TableCell>
                        <TableCell sx={{ fontWeight: 700 }}>Firmware</TableCell>
                        <TableCell sx={{ fontWeight: 700 }}>Time Window</TableCell>
                        <TableCell sx={{ fontWeight: 700 }}>Enabled</TableCell>
                        <TableCell sx={{ fontWeight: 700 }}>Actions</TableCell>
                      </TableRow>
                    </TableHead>
                    <TableBody>
                      {campaigns.map((c) => (
                        <React.Fragment key={c.id}>
                          <TableRow
                            hover
                            onClick={() => handleRowClick(c.id)}
                            sx={{ cursor: 'pointer' }}
                          >
                            <TableCell sx={{ width: 40 }}>
                              <IconButton size="small">
                                {expandedId === c.id ? (
                                  <SvgIcon fontSize="small"><ChevronUpIcon /></SvgIcon>
                                ) : (
                                  <SvgIcon fontSize="small"><ChevronDownIcon /></SvgIcon>
                                )}
                              </IconButton>
                            </TableCell>
                            <TableCell sx={{ fontWeight: 500 }}>{c.vendor || '—'}</TableCell>
                            <TableCell>{c.model || '—'}</TableCell>
                            <TableCell>{c.hw_version || '—'}</TableCell>
                            <TableCell>{getFirmwareLabel(c.firmware_id)}</TableCell>
                            <TableCell>
                              {formatTimeWindow(c.time_window_start, c.time_window_end)}
                            </TableCell>
                            <TableCell onClick={(e) => e.stopPropagation()}>
                              <Switch
                                size="small"
                                checked={c.enabled !== false}
                                onChange={() => handleEnabledToggle(c)}
                              />
                            </TableCell>
                            <TableCell onClick={(e) => e.stopPropagation()}>
                              <Stack direction="row" spacing={0.5}>
                                <IconButton size="small" onClick={() => handleEdit(c)}>
                                  <SvgIcon fontSize="small"><PencilIcon /></SvgIcon>
                                </IconButton>
                                <IconButton
                                  size="small"
                                  color="error"
                                  onClick={() => handleDeleteRequest(c)}
                                >
                                  <SvgIcon fontSize="small"><TrashIcon /></SvgIcon>
                                </IconButton>
                              </Stack>
                            </TableCell>
                          </TableRow>
                          <TableRow>
                            <TableCell colSpan={8} sx={{ py: 0, px: 0 }}>
                              <Collapse in={expandedId === c.id} unmountOnExit>
                                <CampaignLogs campaignId={c.id} />
                              </Collapse>
                            </TableCell>
                          </TableRow>
                        </React.Fragment>
                      ))}
                    </TableBody>
                  </Table>
                )}
              </CardContent>
            </Card>
          </Stack>
        </Container>
      </Box>

      {/* Campaign Create/Edit Dialog */}
      <CampaignDialog
        open={dialogOpen}
        onClose={handleDialogClose}
        onSave={handleSave}
        campaign={editingCampaign}
        firmware={firmware}
        campaigns={campaigns}
      />

      {/* Delete Confirmation Dialog */}
      <Dialog
        open={deleteDialogOpen}
        onClose={() => {
          if (!deleting) {
            setDeleteDialogOpen(false);
            setCampaignToDelete(null);
          }
        }}
        maxWidth="sm"
        fullWidth
      >
        <DialogTitle>Delete Campaign</DialogTitle>
        <DialogContent>
          <Typography>
            Are you sure you want to delete this campaign? This action cannot be undone.
          </Typography>
        </DialogContent>
        <DialogActions>
          <Button
            onClick={() => {
              setDeleteDialogOpen(false);
              setCampaignToDelete(null);
            }}
            disabled={deleting}
          >
            Cancel
          </Button>
          <Button onClick={handleDeleteConfirm} variant="contained" color="error" disabled={deleting}>
            {deleting ? 'Deleting...' : 'Delete'}
          </Button>
        </DialogActions>
      </Dialog>
    </>
  );
};

Page.getLayout = (page) => <DashboardLayout>{page}</DashboardLayout>;

export default Page;
