import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import Head from 'next/head';
import MagnifyingGlassIcon from '@heroicons/react/24/solid/MagnifyingGlassIcon';
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
  InputAdornment,
  Stack,
  OutlinedInput,
  SvgIcon,
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
  const [unsupported, setUnsupported] = useState([]);
  const [commands, setCommands] = useState([]);
  const [auditLogs, setAuditLogs] = useState([]);
  const [whitelistForm, setWhitelistForm] = useState(initialPolicyForm);
  const [loading, setLoading] = useState(false);
  const [selectedUnauthorized, setSelectedUnauthorized] = useState({});
  const [batchResult, setBatchResult] = useState(null);
  const [selectedPolicies, setSelectedPolicies] = useState({});
  const [batchDeleteOpen, setBatchDeleteOpen] = useState(false);
  const [deletingSn, setDeletingSn] = useState(null);
  const [policySearch, setPolicySearch] = useState('');
  const [clearAuditOpen, setClearAuditOpen] = useState(false);
  const [unsupportedActionSn, setUnsupportedActionSn] = useState(null);

 const whitelistFileRef = useRef(null);

  const fetchData = useCallback(async () => {
    setLoading(true);
    try {
      const [cfg, policyResp, unauthorizedResp, unsupportedResp, commandsResp, auditResp] = await Promise.all([
        httpRequest(`${apiPrefix}/lock/config`, 'GET'),
        httpRequest(`${apiPrefix}/lock/policies`, 'GET'),
        httpRequest(`${apiPrefix}/lock/unauthorized`, 'GET'),
        httpRequest(`${apiPrefix}/lock/unsupported`, 'GET'),
        httpRequest(`${apiPrefix}/lock/commands`, 'GET'),
        httpRequest(`${apiPrefix}/lock/audit`, 'GET'),
      ]);

      if (cfg.status === 200 && cfg.result) setConfig(cfg.result);
      if (policyResp.status === 200 && Array.isArray(policyResp.result)) setPolicies(policyResp.result);
      if (unauthorizedResp.status === 200 && Array.isArray(unauthorizedResp.result)) setUnauthorized(unauthorizedResp.result);
      if (unsupportedResp.status === 200 && Array.isArray(unsupportedResp.result)) setUnsupported(unsupportedResp.result);
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

  const submitPolicy = async () => {
    const body = JSON.stringify(whitelistForm);
    const { status } = await httpRequest(`${apiPrefix}/lock/whitelist`, 'POST', body);
    if (status === 200) {
      setAlert({ severity: 'success', message: 'Whitelist policy saved.' });
      setWhitelistForm(initialPolicyForm);
      fetchData();
    }
  };

  const uploadBatchCSV = async (file) => {
    if (!file) return;
    const token = localStorage.getItem('token');
    const response = await fetch(`${apiPrefix}/lock/whitelist/batch`, {
      method: 'POST',
      headers: {
        Authorization: token,
        'Content-Type': 'text/csv',
      },
      body: file,
    });
    const result = await response.json();
    setBatchResult({ status: response.status, result });
    if (response.ok || response.status === 207) {
      setAlert({
        severity: response.status === 207 ? 'warning' : 'success',
        message: `Batch import: created ${result.created ?? 0}, errors ${result.errors?.length ?? 0}.`,
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
    setDeletingSn(sn);
    try {
      const { status } = await httpRequest(`${apiPrefix}/lock/policies/${encodeURIComponent(sn)}`, 'DELETE');
      if (status >= 200 && status < 300) {
        setPolicies((prev) => prev.filter((p) => p.sn !== sn));
        setUnauthorized((prev) => prev.filter((u) => u.sn !== sn));
        setAlert({ severity: 'success', message: 'Policy deleted.' });
      }
   } finally {
     setDeletingSn(null);
   }
 };

  const clearAudit = async () => {
    setClearAuditOpen(false);
    const { status, result } = await httpRequest(`${apiPrefix}/lock/audit`, 'DELETE');
    if (status >= 200 && status < 300) {
      setAuditLogs([]);
      setAlert({ severity: 'success', message: `Cleared ${result?.deleted ?? 0} audit record(s).` });
      fetchData().catch(() => {});
    } else {
      setAlert({ severity: 'error', message: result?.error || 'Failed to clear audit history.' });
    }
  };

 const filteredPolicies = useMemo(() => {
    const q = policySearch.trim().toLowerCase();
    if (!q) return policies;
    return policies.filter((p) => [
      p.sn,
      p.policy_type,
      p.allowed_ip_range,
      p.reason_code,
      p.description,
    ].some((v) => String(v ?? '').toLowerCase().includes(q)));
  }, [policies, policySearch]);

  const selectedPolicyCount = Object.values(selectedPolicies).filter(Boolean).length;

  const togglePolicy = (sn) => {
    setSelectedPolicies((prev) => ({ ...prev, [sn]: !prev[sn] }));
  };

  const allPoliciesSelected = filteredPolicies.length > 0 && filteredPolicies.every((p) => selectedPolicies[p.sn]);

  const toggleAllPolicies = () => {
    if (allPoliciesSelected) {
      setSelectedPolicies((prev) => {
        const next = { ...prev };
        filteredPolicies.forEach((p) => { delete next[p.sn]; });
        return next;
      });
    } else {
      const next = { ...selectedPolicies };
      filteredPolicies.forEach((p) => { next[p.sn] = true; });
      setSelectedPolicies(next);
    }
  };

  const doBatchDeletePolicies = async () => {
    const sns = policies.filter((p) => selectedPolicies[p.sn]).map((p) => p.sn);
    if (sns.length === 0) return;
    const body = JSON.stringify({ sns });
    const { status, result } = await httpRequest(`${apiPrefix}/lock/policies/batch-delete`, 'POST', body);
    setBatchDeleteOpen(false);
    if (status >= 200 && status < 300) {
      const deletedSet = new Set(sns);
      setPolicies((prev) => prev.filter((p) => !deletedSet.has(p.sn)));
      setUnauthorized((prev) => prev.filter((u) => !deletedSet.has(u.sn)));
      setAlert({
        severity: status === 207 ? 'warning' : 'success',
        message: `Deleted ${result.deleted ?? sns.length} policy/policies${result.errors?.length ? `, ${result.errors.length} error(s)` : ''}.`,
      });
      setSelectedPolicies({});
      fetchData().catch(() => {});
    } else {
      setAlert({ severity: 'error', message: result?.error || 'Batch delete failed.' });
    }
  };

  const toggleUnauthorized = (sn) => {
    setSelectedUnauthorized((prev) => ({ ...prev, [sn]: !prev[sn] }));
  };

  const optOutUnsupported = async (sn) => {
    setUnsupportedActionSn(sn);
    try {
      const { status } = await httpRequest(
        `${apiPrefix}/lock/unsupported/${encodeURIComponent(sn)}/opt-out`,
        'POST'
      );
      if (status >= 200 && status < 300) {
        setAlert({ severity: 'success', message: 'Stopped detecting this device.' });
        fetchData();
      }
    } finally {
      setUnsupportedActionSn(null);
    }
  };

  const resumeUnsupported = async (sn) => {
    setUnsupportedActionSn(sn);
    try {
      const { status } = await httpRequest(
        `${apiPrefix}/lock/unsupported/${encodeURIComponent(sn)}/opt-out`,
        'DELETE'
      );
      if (status >= 200 && status < 300) {
        setAlert({ severity: 'success', message: 'Resumed detecting this device.' });
        fetchData();
      }
    } finally {
      setUnsupportedActionSn(null);
    }
  };

  const renderPolicyRows = () => {
    if (filteredPolicies.length === 0) {
      return (
        <TableRow>
          <TableCell colSpan={7}>
            {policies.length === 0 ? 'No lock policies found.' : 'No policies match your search.'}
          </TableCell>
        </TableRow>
      );
    }
    return filteredPolicies.map((policy) => (
      <TableRow key={policy.sn}>
        <TableCell padding="checkbox">
          <Checkbox
            checked={Boolean(selectedPolicies[policy.sn])}
            onChange={() => togglePolicy(policy.sn)}
          />
        </TableCell>
        <TableCell>{policy.sn}</TableCell>
        <TableCell>{policy.policy_type}</TableCell>
        <TableCell>{policy.allowed_ip_range || '-'}</TableCell>
        <TableCell>{policy.reason_code || '-'}</TableCell>
        <TableCell>{policy.description || '-'}</TableCell>
        <TableCell align="right">
          <Button color="error" size="small" disabled={deletingSn === policy.sn} onClick={() => deletePolicy(policy.sn)}>
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

            <Card>
              <CardHeader title="Add Whitelist" />
              <Divider />
              <CardContent>
                <Stack spacing={2}>
                  <TextField
                    label="SN"
                    value={whitelistForm.sn}
                    onChange={(event) => setWhitelistForm((prev) => ({ ...prev, sn: event.target.value }))}
                    required
                    fullWidth
                  />
                  <TextField
                    label="Allowed IP Range (CIDR)"
                    value={whitelistForm.allowed_ip_range}
                    onChange={(event) => setWhitelistForm((prev) => ({ ...prev, allowed_ip_range: event.target.value }))}
                    placeholder="10.10.0.0/16"
                    required
                    fullWidth
                  />
                  <TextField
                    label="Description"
                    value={whitelistForm.description}
                    onChange={(event) => setWhitelistForm((prev) => ({ ...prev, description: event.target.value }))}
                    fullWidth
                    multiline
                    minRows={2}
                  />
                  <Button variant="contained" onClick={submitPolicy} disabled={!whitelistForm.sn || !whitelistForm.allowed_ip_range}>
                    Save
                  </Button>
                  <Button variant="outlined" onClick={() => whitelistFileRef.current?.click()}>
                    Import CSV
                  </Button>
                  <input
                    ref={whitelistFileRef}
                    type="file"
                    accept=".csv,text/csv"
                    hidden
                    onChange={(event) => {
                      uploadBatchCSV(event.target.files?.[0]);
                      event.target.value = '';
                    }}
                  />
                </Stack>
              </CardContent>
            </Card>

            {batchResult && (
              <Card>
                <CardHeader title="Batch Import Result" />
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
              <CardHeader
                title="Policies"
                action={
                 <Stack direction="row" spacing={1} alignItems="center">
                    <OutlinedInput
                      size="small"
                      value={policySearch}
                      onChange={(event) => setPolicySearch(event.target.value)}
                      placeholder="Search SN / IP / description"
                      startAdornment={(
                        <InputAdornment position="start">
                          <SvgIcon
                            color="action"
                            fontSize="small"
                          >
                            <MagnifyingGlassIcon />
                          </SvgIcon>
                        </InputAdornment>
                      )}
                      sx={{ width: 260 }}
                    />
                    {selectedPolicyCount > 0 ? (
                      <Button color="error" size="small" variant="outlined" onClick={() => setBatchDeleteOpen(true)}>
                        Delete Selected ({selectedPolicyCount})
                      </Button>
                    ) : null}
                  </Stack>
                }
              />
              <Divider />
              <CardContent sx={{ p: 0 }}>
                <Table>
                  <TableHead>
                    <TableRow>
                      <TableCell padding="checkbox">
                        <Checkbox
                          checked={allPoliciesSelected}
                          indeterminate={selectedPolicyCount > 0 && !allPoliciesSelected}
                          onChange={toggleAllPolicies}
                        />
                      </TableCell>
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

            <Stack direction={{ xs: 'column', lg: 'row' }} spacing={3}>
              <Card sx={{ flex: 1 }}>
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
              <SimpleTable
                sx={{ flex: 1 }}
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
            </Stack>

            <Card>
              <CardHeader title="Unsupported Devices" />
              <Divider />
              <CardContent sx={{ p: 0 }}>
                <Table>
                  <TableHead>
                    <TableRow>
                      <TableCell>SN</TableCell>
                      <TableCell>Reason</TableCell>
                      <TableCell>Detail</TableCell>
                      <TableCell>Last Checked</TableCell>
                      <TableCell>Opt-out</TableCell>
                      <TableCell align="right">Actions</TableCell>
                    </TableRow>
                  </TableHead>
                  <TableBody>
                    {unsupported.length === 0 ? (
                      <TableRow>
                        <TableCell colSpan={6}>No unsupported devices.</TableCell>
                      </TableRow>
                    ) : (
                      unsupported.map((item) => (
                        <TableRow key={item.sn}>
                          <TableCell>{item.sn}</TableCell>
                          <TableCell>{item.reason || '-'}</TableCell>
                          <TableCell>{item.detail || '-'}</TableCell>
                          <TableCell>
                            {item.last_checked_at ? new Date(item.last_checked_at).toLocaleString() : '-'}
                          </TableCell>
                          <TableCell>
                            <Chip
                              label={item.opt_out ? 'Yes' : 'No'}
                              color={item.opt_out ? 'default' : 'warning'}
                              size="small"
                            />
                          </TableCell>
                          <TableCell align="right">
                            {item.opt_out ? (
                              <Button
                                size="small"
                                variant="outlined"
                                disabled={unsupportedActionSn === item.sn}
                                onClick={() => resumeUnsupported(item.sn)}
                              >
                                Resume
                              </Button>
                            ) : (
                              <Button
                                size="small"
                                variant="outlined"
                                disabled={unsupportedActionSn === item.sn}
                                onClick={() => optOutUnsupported(item.sn)}
                              >
                                Stop detecting
                              </Button>
                            )}
                          </TableCell>
                        </TableRow>
                      ))
                    )}
                  </TableBody>
                </Table>
              </CardContent>
            </Card>

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
             action={
               auditLogs.length > 0 ? (
                 <Button color="error" size="small" variant="outlined" onClick={() => setClearAuditOpen(true)}>
                   Clear
                 </Button>
               ) : null
             }
           />
          </Stack>
        </Container>
      </Box>

      <Dialog open={batchDeleteOpen} onClose={() => setBatchDeleteOpen(false)}>
        <DialogTitle>Delete {selectedPolicyCount} Policy/Policies?</DialogTitle>
        <DialogContent>
          <DialogContentText>
            This will permanently remove the selected lock policies. Affected devices will be re-evaluated for lock status.
          </DialogContentText>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setBatchDeleteOpen(false)}>Cancel</Button>
         <Button color="error" variant="contained" onClick={doBatchDeletePolicies}>
           Delete
         </Button>
       </DialogActions>
     </Dialog>
      <Dialog open={clearAuditOpen} onClose={() => setClearAuditOpen(false)}>
        <DialogTitle>Clear Audit History?</DialogTitle>
        <DialogContent>
          <DialogContentText>
            This will permanently remove all lock audit records for this tenant. The clear action itself will be logged.
          </DialogContentText>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setClearAuditOpen(false)}>Cancel</Button>
          <Button color="error" variant="contained" onClick={clearAudit}>
            Clear
          </Button>
        </DialogActions>
      </Dialog>
   </>
  );
};

const SimpleTable = ({ title, columns, rows, empty, sx, action }) => (
  <Card sx={sx}>
    <CardHeader title={title} action={action} />
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
