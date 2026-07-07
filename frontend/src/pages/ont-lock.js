import { useCallback, useEffect, useRef, useState } from 'react';
import Head from 'next/head';
import {
  Box,
  Button,
  Card,
  CardContent,
  CardHeader,
  Checkbox,
  Chip,
  Container,
  Dialog,
  DialogActions,
  DialogContent,
  DialogContentText,
  DialogTitle,
  Divider,
  FormControlLabel,
  Grid,
  Stack,
  Switch,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableRow,
  TextField,
  Typography,
} from '@mui/material';
import { Layout as DashboardLayout } from 'src/layouts/dashboard/layout';
import { useAlertContext } from 'src/contexts/error-context';
import { useBackendContext } from 'src/contexts/backend-context';

const initialPolicyForm = {
  sn: '',
  allowed_ip_range: '',
  reason_code: '',
  description: '',
};

const statusColor = (status) => {
  switch (status) {
    case 'LOCKED':
      return 'error';
    case 'UNLOCKED':
      return 'success';
    case 'PENDING':
      return 'warning';
    default:
      return 'default';
  }
};

const Page = () => {
  const { httpRequest, apiPrefix } = useBackendContext();
  const { setAlert } = useAlertContext();

  const [config, setConfig] = useState({ master_enabled: true, auto_lock_enabled: true });
  const [policies, setPolicies] = useState([]);
  const [unauthorized, setUnauthorized] = useState([]);
  const [commands, setCommands] = useState([]);
  const [auditLogs, setAuditLogs] = useState([]);
  const [whitelistForm, setWhitelistForm] = useState(initialPolicyForm);
  const [blacklistForm, setBlacklistForm] = useState(initialPolicyForm);
  const [loading, setLoading] = useState(false);
  const [selectedUnauthorized, setSelectedUnauthorized] = useState({});
  const [batchResult, setBatchResult] = useState(null);
  const [confirmDialog, setConfirmDialog] = useState({ open: false, type: null, body: null, forcePath: null });

  const whitelistFileRef = useRef(null);
  const blacklistFileRef = useRef(null);

  const fetchData = useCallback(async () => {
    setLoading(true);
    try {
      const [cfg, policyResp, unauthorizedResp, commandsResp, auditResp] = await Promise.all([
        httpRequest(`${apiPrefix}/lock/config`, 'GET'),
        httpRequest(`${apiPrefix}/lock/policies`, 'GET'),
        httpRequest(`${apiPrefix}/lock/unauthorized`, 'GET'),
        httpRequest(`${apiPrefix}/lock/commands`, 'GET'),
        httpRequest(`${apiPrefix}/lock/audit`, 'GET'),
      ]);

      if (cfg.status === 200 && cfg.result) setConfig(cfg.result);
      if (policyResp.status === 200 && Array.isArray(policyResp.result)) setPolicies(policyResp.result);
      if (unauthorizedResp.status === 200 && Array.isArray(unauthorizedResp.result)) setUnauthorized(unauthorizedResp.result);
      if (commandsResp.status === 200 && Array.isArray(commandsResp.result)) setCommands(commandsResp.result);
      if (auditResp.status === 200 && Array.isArray(auditResp.result)) setAuditLogs(auditResp.result);
    } finally {
      setLoading(false);
    }
  }, [apiPrefix, httpRequest]);

  useEffect(() => {
    fetchData();
  }, [fetchData]);

  const updateConfig = async (nextConfig) => {
    const body = JSON.stringify({
      id: 'global',
      master_enabled: nextConfig.master_enabled,
      auto_lock_enabled: nextConfig.auto_lock_enabled,
    });
    const { status, result } = await httpRequest(`${apiPrefix}/lock/config`, 'PUT', body);
    if (status === 200 && result) {
      setConfig(result);
      setAlert({ severity: 'success', message: 'ONT Lock configuration updated.' });
    }
  };

  const postPolicy = async (type, form, force = false) => {
    const path = type === 'whitelist' ? '/lock/whitelist' : '/lock/blacklist';
    const query = force ? '?force=true' : '';
    const body = JSON.stringify(form);
    const { status, result } = await httpRequest(`${apiPrefix}${path}${query}`, 'POST', body);
    if (status === 409 && result?.existing_policy) {
      setConfirmDialog({
        open: true,
        type,
        body: form,
        existing: result.existing_policy,
      });
      return false;
    }
    if (status === 200) {
      setAlert({ severity: 'success', message: `${type === 'whitelist' ? 'Whitelist' : 'Blacklist'} policy saved.` });
      return true;
    }
    return false;
  };

  const submitPolicy = async (type) => {
    const form = type === 'whitelist' ? whitelistForm : blacklistForm;
    const ok = await postPolicy(type, form);
    if (ok) {
      if (type === 'whitelist') setWhitelistForm(initialPolicyForm);
      else setBlacklistForm(initialPolicyForm);
      fetchData();
    }
  };

  const confirmOverride = async () => {
    const { type, body } = confirmDialog;
    setConfirmDialog({ open: false, type: null, body: null });
    const ok = await postPolicy(type, body, true);
    if (ok) {
      if (type === 'whitelist') setWhitelistForm(initialPolicyForm);
      else setBlacklistForm(initialPolicyForm);
      fetchData();
    }
  };

  const uploadBatchCSV = async (type, file) => {
    if (!file) return;
    const path = type === 'whitelist' ? '/lock/whitelist/batch' : '/lock/blacklist/batch';
    const token = localStorage.getItem('token');
    const response = await fetch(`${apiPrefix}${path}`, {
      method: 'POST',
      headers: {
        Authorization: token,
        'Content-Type': 'text/csv',
      },
      body: file,
    });
    const result = await response.json();
    setBatchResult({ type, status: response.status, result });
    if (response.ok || response.status === 207) {
      setAlert({
        severity: response.status === 207 ? 'warning' : 'success',
        message: `Batch ${type}: created ${result.created ?? 0}, errors ${result.errors?.length ?? 0}.`,
      });
      fetchData();
    } else {
      setAlert({ severity: 'error', message: result.error || 'Batch import failed.' });
    }
  };

  const batchWhitelistFromUnauthorized = async () => {
    const selected = unauthorized.filter((item) => selectedUnauthorized[item.sn]);
    if (selected.length === 0) {
      setAlert({ severity: 'warning', message: 'Select at least one unauthorized device.' });
      return;
    }
    const body = JSON.stringify({
      remove_from_unauthorized: true,
      items: selected.map((item) => ({
        sn: item.sn,
        reported_ip: item.reported_ip,
        allowed_ip_range: item.reported_ip ? `${item.reported_ip}/32` : '',
        description: 'Whitelisted from unauthorized list',
      })),
    });
    const { status, result } = await httpRequest(`${apiPrefix}/lock/unauthorized/batch-whitelist`, 'POST', body);
    if (status === 202 || status === 207) {
      setAlert({
        severity: status === 207 ? 'warning' : 'success',
        message: `Whitelisted ${result.created ?? 0} device(s).`,
      });
      setSelectedUnauthorized({});
      fetchData();
    }
  };

  const deletePolicy = async (sn) => {
    const { status } = await httpRequest(`${apiPrefix}/lock/policies/${encodeURIComponent(sn)}`, 'DELETE');
    if (status === 204) {
      setAlert({ severity: 'success', message: 'Policy deleted.' });
      fetchData();
    }
  };

  const toggleUnauthorized = (sn) => {
    setSelectedUnauthorized((prev) => ({ ...prev, [sn]: !prev[sn] }));
  };

  const renderPolicyRows = () => {
    if (policies.length === 0) {
      return (
        <TableRow>
          <TableCell colSpan={6}>No lock policies found.</TableCell>
        </TableRow>
      );
    }
    return policies.map((policy) => (
      <TableRow key={policy.sn}>
        <TableCell>{policy.sn}</TableCell>
        <TableCell>{policy.policy_type}</TableCell>
        <TableCell>{policy.allowed_ip_range || '-'}</TableCell>
        <TableCell>{policy.reason_code || '-'}</TableCell>
        <TableCell>{policy.description || '-'}</TableCell>
        <TableCell align="right">
          <Button color="error" size="small" onClick={() => deletePolicy(policy.sn)}>
            Delete
          </Button>
        </TableCell>
      </TableRow>
    ));
  };

  return (
    <>
      <Head>
        <title>ONT Lock | Oktopus</title>
      </Head>
      <Box component="main" sx={{ flexGrow: 1, py: 8 }}>
        <Container maxWidth="xl">
          <Stack spacing={3}>
            <Stack direction="row" justifyContent="space-between" alignItems="center">
              <Box>
                <Typography variant="h4">ONT Lock</Typography>
                <Typography color="text.secondary" variant="body2">
                  SN and WAN IP based device admission control for Telkomsel.
                </Typography>
              </Box>
              <Button disabled={loading} onClick={fetchData} variant="outlined">
                Refresh
              </Button>
            </Stack>

            <Card>
              <CardHeader title="Global Configuration" />
              <Divider />
              <CardContent>
                <Stack direction={{ xs: 'column', md: 'row' }} spacing={3}>
                  <FormControlLabel
                    control={
                      <Switch
                        checked={Boolean(config.master_enabled)}
                        onChange={(event) => {
                          const next = { ...config, master_enabled: event.target.checked };
                          setConfig(next);
                          updateConfig(next);
                        }}
                      />
                    }
                    label="Master lock switch"
                  />
                  <FormControlLabel
                    control={
                      <Switch
                        checked={Boolean(config.auto_lock_enabled)}
                        onChange={(event) => {
                          const next = { ...config, auto_lock_enabled: event.target.checked };
                          setConfig(next);
                          updateConfig(next);
                        }}
                      />
                    }
                    label="Auto lock unauthorized devices"
                  />
                </Stack>
              </CardContent>
            </Card>

            <Grid container spacing={3}>
              <Grid item xs={12} md={6}>
                <PolicyForm
                  title="Add Whitelist"
                  form={whitelistForm}
                  setForm={setWhitelistForm}
                  onSubmit={() => submitPolicy('whitelist')}
                  showCIDR
                />
                <Stack direction="row" spacing={1} sx={{ mt: 2 }}>
                  <Button variant="outlined" onClick={() => whitelistFileRef.current?.click()}>
                    Import CSV
                  </Button>
                  <input
                    ref={whitelistFileRef}
                    type="file"
                    accept=".csv,text/csv"
                    hidden
                    onChange={(event) => {
                      uploadBatchCSV('whitelist', event.target.files?.[0]);
                      event.target.value = '';
                    }}
                  />
                </Stack>
              </Grid>
              <Grid item xs={12} md={6}>
                <PolicyForm
                  title="Add Blacklist"
                  form={blacklistForm}
                  setForm={setBlacklistForm}
                  onSubmit={() => submitPolicy('blacklist')}
                  showReason
                />
                <Stack direction="row" spacing={1} sx={{ mt: 2 }}>
                  <Button variant="outlined" onClick={() => blacklistFileRef.current?.click()}>
                    Import CSV
                  </Button>
                  <input
                    ref={blacklistFileRef}
                    type="file"
                    accept=".csv,text/csv"
                    hidden
                    onChange={(event) => {
                      uploadBatchCSV('blacklist', event.target.files?.[0]);
                      event.target.value = '';
                    }}
                  />
                </Stack>
              </Grid>
            </Grid>

            {batchResult && (
              <Card>
                <CardHeader title={`Batch Import (${batchResult.type})`} />
                <Divider />
                <CardContent>
                  <Typography variant="body2">
                    Created: {batchResult.result?.created ?? 0}
                    {batchResult.result?.errors?.length ? ` | Errors: ${batchResult.result.errors.length}` : ''}
                  </Typography>
                  {batchResult.result?.errors?.map((err, idx) => (
                    <Typography key={idx} color="error" variant="body2">
                      {err}
                    </Typography>
                  ))}
                </CardContent>
              </Card>
            )}

            <Card>
              <CardHeader title="Policies" />
              <Divider />
              <CardContent sx={{ p: 0 }}>
                <Table>
                  <TableHead>
                    <TableRow>
                      <TableCell>SN</TableCell>
                      <TableCell>Type</TableCell>
                      <TableCell>Allowed IP Range</TableCell>
                      <TableCell>Reason</TableCell>
                      <TableCell>Description</TableCell>
                      <TableCell align="right">Actions</TableCell>
                    </TableRow>
                  </TableHead>
                  <TableBody>{renderPolicyRows()}</TableBody>
                </Table>
              </CardContent>
            </Card>

            <Grid container spacing={3}>
              <Grid item xs={12} lg={6}>
                <Card>
                  <CardHeader
                    title="Unauthorized Devices"
                    action={
                      <Button size="small" variant="contained" onClick={batchWhitelistFromUnauthorized}>
                        Whitelist Selected
                      </Button>
                    }
                  />
                  <Divider />
                  <CardContent sx={{ p: 0 }}>
                    <Table>
                      <TableHead>
                        <TableRow>
                          <TableCell padding="checkbox" />
                          <TableCell>SN</TableCell>
                          <TableCell>Reported IP</TableCell>
                          <TableCell>Status</TableCell>
                          <TableCell>Reason</TableCell>
                          <TableCell>Last Seen</TableCell>
                        </TableRow>
                      </TableHead>
                      <TableBody>
                        {unauthorized.length === 0 ? (
                          <TableRow>
                            <TableCell colSpan={6}>No unauthorized devices.</TableCell>
                          </TableRow>
                        ) : (
                          unauthorized.map((item) => (
                            <TableRow key={item.sn}>
                              <TableCell padding="checkbox">
                                <Checkbox
                                  checked={Boolean(selectedUnauthorized[item.sn])}
                                  onChange={() => toggleUnauthorized(item.sn)}
                                />
                              </TableCell>
                              <TableCell>{item.sn}</TableCell>
                              <TableCell>{item.reported_ip || '-'}</TableCell>
                              <TableCell>
                                <Chip label={item.status} color={statusColor(item.status)} size="small" />
                              </TableCell>
                              <TableCell>{item.reason}</TableCell>
                              <TableCell>{item.last_seen ? new Date(item.last_seen).toLocaleString() : '-'}</TableCell>
                            </TableRow>
                          ))
                        )}
                      </TableBody>
                    </Table>
                  </CardContent>
                </Card>
              </Grid>
              <Grid item xs={12} lg={6}>
                <SimpleTable
                  title="Recent Commands"
                  columns={['SN', 'Target', 'Status', 'Command ID', 'Updated']}
                  rows={commands.map((item) => [
                    item.device_sn,
                    item.target_status,
                    <Chip
                      key="status"
                      label={item.status}
                      color={item.status === 'success' ? 'success' : item.status === 'failed' ? 'error' : 'warning'}
                      size="small"
                    />,
                    item.command_id,
                    item.updated_at ? new Date(item.updated_at).toLocaleString() : '-',
                  ])}
                  empty="No lock commands yet."
                />
              </Grid>
            </Grid>

            <SimpleTable
              title="Audit History"
              columns={['Action', 'SN', 'Status', 'Operator', 'Created']}
              rows={auditLogs.map((item) => [
                item.action,
                item.sn || '-',
                item.status || '-',
                item.operator_id || '-',
                item.created_at ? new Date(item.created_at).toLocaleString() : '-',
              ])}
              empty="No audit logs yet."
            />
          </Stack>
        </Container>
      </Box>

      <Dialog open={confirmDialog.open} onClose={() => setConfirmDialog({ open: false })}>
        <DialogTitle>Confirm Policy Override</DialogTitle>
        <DialogContent>
          <DialogContentText>
            Device {confirmDialog.body?.sn} is currently on the whitelist
            {confirmDialog.existing?.allowed_ip_range ? ` (${confirmDialog.existing.allowed_ip_range})` : ''}.
            Adding it to the blacklist will override the whitelist. Continue?
          </DialogContentText>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setConfirmDialog({ open: false })}>Cancel</Button>
          <Button color="error" variant="contained" onClick={confirmOverride}>
            Override
          </Button>
        </DialogActions>
      </Dialog>
    </>
  );
};

const PolicyForm = ({ title, form, setForm, onSubmit, showCIDR, showReason }) => (
  <Card>
    <CardHeader title={title} />
    <Divider />
    <CardContent>
      <Stack spacing={2}>
        <TextField
          label="SN"
          value={form.sn}
          onChange={(event) => setForm((prev) => ({ ...prev, sn: event.target.value }))}
          required
          fullWidth
        />
        {showCIDR && (
          <TextField
            label="Allowed IP Range (CIDR)"
            value={form.allowed_ip_range}
            onChange={(event) => setForm((prev) => ({ ...prev, allowed_ip_range: event.target.value }))}
            placeholder="10.10.0.0/16"
            required
            fullWidth
          />
        )}
        {showReason && (
          <TextField
            label="Reason"
            value={form.reason_code}
            onChange={(event) => setForm((prev) => ({ ...prev, reason_code: event.target.value }))}
            placeholder="lost, unpaid, churn"
            fullWidth
          />
        )}
        <TextField
          label="Description"
          value={form.description}
          onChange={(event) => setForm((prev) => ({ ...prev, description: event.target.value }))}
          fullWidth
          multiline
          minRows={2}
        />
        <Button variant="contained" onClick={onSubmit} disabled={!form.sn || (showCIDR && !form.allowed_ip_range)}>
          Save
        </Button>
      </Stack>
    </CardContent>
  </Card>
);

const SimpleTable = ({ title, columns, rows, empty }) => (
  <Card>
    <CardHeader title={title} />
    <Divider />
    <CardContent sx={{ p: 0 }}>
      <Table>
        <TableHead>
          <TableRow>
            {columns.map((column) => (
              <TableCell key={column}>{column}</TableCell>
            ))}
          </TableRow>
        </TableHead>
        <TableBody>
          {rows.length === 0 ? (
            <TableRow>
              <TableCell colSpan={columns.length}>{empty}</TableCell>
            </TableRow>
          ) : (
            rows.map((row, idx) => (
              <TableRow key={idx}>
                {row.map((cell, cellIdx) => (
                  <TableCell key={cellIdx}>{cell}</TableCell>
                ))}
              </TableRow>
            ))
          )}
        </TableBody>
      </Table>
    </CardContent>
  </Card>
);

Page.getLayout = (page) => <DashboardLayout>{page}</DashboardLayout>;

export default Page;
