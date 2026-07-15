import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import Head from 'next/head';
import { useRouter } from 'next/router';
import MagnifyingGlassIcon from '@heroicons/react/24/solid/MagnifyingGlassIcon';
import ArrowDownTrayIcon from '@heroicons/react/24/solid/ArrowDownTrayIcon';
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
  Tab,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TablePagination,
  TableRow,
  Tabs,
  TextField,
  Tooltip,
  Typography,
} from '@mui/material';
import { Layout as DashboardLayout } from 'src/layouts/dashboard/layout';
import { useAlertContext } from 'src/contexts/error-context';
import { useBackendContext } from 'src/contexts/backend-context';
import { OverviewKpis } from 'src/sections/ont-lock/overview-kpis';
import { CommandsStatusChart } from 'src/sections/ont-lock/commands-status-chart';

const TAB_KEYS = ['overview', 'policies', 'exceptions', 'activity'];

/** Legacy ?tab=queue bookmarks map to Exceptions. */
const normalizeTab = (raw) => {
  const t = String(raw || 'overview').toLowerCase();
  if (t === 'queue') return 'exceptions';
  return TAB_KEYS.includes(t) ? t : 'overview';
};

function hostCIDR(ip) {
  const trimmed = (ip || '').trim();
  if (!trimmed) return trimmed;
  return trimmed.includes(':') ? `${trimmed}/128` : `${trimmed}/32`;
}

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

/** Map lock-engine reason codes to operator-facing labels (Exceptions / Unauthorized). */
const formatUnauthorizedReason = (reason) => {
  switch (String(reason || '').toUpperCase()) {
    case 'UNAUTHORIZED':
      return 'Not in whitelist / IP outside allowed range';
    case 'INVALID_IP':
      return 'Invalid or missing WAN IP';
    case 'MASTER_DISABLED':
      return 'Master lock switch is off';
    case 'AUTHORIZED':
      return 'Authorized by whitelist';
    case '':
      return '—';
    default:
      return String(reason);
  }
};

const truncateCell = (value, max = 28) => {
  const text = value == null || value === '' ? '—' : String(value);
  if (text.length <= max) {
    return { display: text, full: text, truncated: false };
  }
  return { display: `${text.slice(0, max)}…`, full: text, truncated: true };
};

const TruncatedCell = ({ value, max = 28, sx }) => {
  const { display, full, truncated } = truncateCell(value, max);
  const cell = (
    <TableCell
      sx={{
        maxWidth: max * 8,
        overflow: 'hidden',
        textOverflow: 'ellipsis',
        whiteSpace: 'nowrap',
        ...sx,
      }}
    >
      {display}
    </TableCell>
  );
  if (!truncated) {
    return cell;
  }
  return (
    <Tooltip title={full} placement="top-start">
      {cell}
    </Tooltip>
  );
};

const downloadCsv = (filename, headers, rows) => {
  const escape = (v) => {
    const s = v == null ? '' : String(v);
    if (/[",\n\r]/.test(s)) {
      return `"${s.replace(/"/g, '""')}"`;
    }
    return s;
  };
  const lines = [
    headers.map(escape).join(','),
    ...rows.map((row) => row.map(escape).join(',')),
  ];
  const blob = new Blob([lines.join('\n')], { type: 'text/csv;charset=utf-8' });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = filename;
  a.click();
  URL.revokeObjectURL(url);
};

const parseListResponse = (data) => {
  if (Array.isArray(data)) {
    return { items: data, total: data.length, page: 0, size: data.length };
  }
  return {
    items: data?.items || [],
    total: data?.total ?? 0,
    page: data?.page ?? 0,
    size: data?.size ?? 25,
  };
};

const aggregateCommandStatuses = (commands) => {
  const order = ['success', 'failed', 'pending', 'retry', 'other'];
  const counts = { success: 0, failed: 0, pending: 0, retry: 0, other: 0 };
  commands.forEach((c) => {
    const s = String(c.status || '').toLowerCase();
    if (Object.prototype.hasOwnProperty.call(counts, s) && s !== 'other') {
      counts[s] += 1;
    } else {
      counts.other += 1;
    }
  });
  return {
    labels: order.map((k) => k.charAt(0).toUpperCase() + k.slice(1)),
    values: order.map((k) => counts[k]),
    success: counts.success,
    failed: counts.failed,
  };
};

const Page = () => {
  const { httpRequest, apiPrefix } = useBackendContext();
  const { setAlert } = useAlertContext();
  const router = useRouter();

  const activeTab = useMemo(
    () => normalizeTab(router.query.tab),
    [router.query.tab]
  );

  // Rewrite legacy ?tab=queue to ?tab=exceptions (shallow).
  useEffect(() => {
    if (String(router.query.tab || '').toLowerCase() !== 'queue') return;
    router.replace(
      { pathname: router.pathname, query: { ...router.query, tab: 'exceptions' } },
      undefined,
      { shallow: true }
    );
  }, [router]);

  const [config, setConfig] = useState({ master_enabled: true, auto_lock_enabled: true });
  const [policies, setPolicies] = useState([]);
  const [unauthorized, setUnauthorized] = useState([]);
  const [unsupported, setUnsupported] = useState([]);
  const [commands, setCommands] = useState([]);
  const [commandsTotal, setCommandsTotal] = useState(0);
  const [auditLogs, setAuditLogs] = useState([]);
  const [auditTotal, setAuditTotal] = useState(0);
  const [chartCommands, setChartCommands] = useState([]);
  const [whitelistForm, setWhitelistForm] = useState(initialPolicyForm);
  const [loading, setLoading] = useState(false);
  const [selectedUnauthorized, setSelectedUnauthorized] = useState({});
  const [batchResult, setBatchResult] = useState(null);
  const [selectedPolicies, setSelectedPolicies] = useState({});
  const [batchDeleteOpen, setBatchDeleteOpen] = useState(false);
  const [unauthDismissOpen, setUnauthDismissOpen] = useState(false);
  const [deletingSn, setDeletingSn] = useState(null);
  const [policySearch, setPolicySearch] = useState('');
  const [policyPage, setPolicyPage] = useState(0);
  const [policyRowsPerPage, setPolicyRowsPerPage] = useState(25);
  const [unauthSearch, setUnauthSearch] = useState('');
  const [unauthPage, setUnauthPage] = useState(0);
  const [unauthRowsPerPage, setUnauthRowsPerPage] = useState(25);
  const [cmdSearch, setCmdSearch] = useState('');
  const [cmdSearchApplied, setCmdSearchApplied] = useState('');
  const [cmdPage, setCmdPage] = useState(0);
  const [cmdRowsPerPage, setCmdRowsPerPage] = useState(25);
  const [auditSearch, setAuditSearch] = useState('');
  const [auditSearchApplied, setAuditSearchApplied] = useState('');
  const [auditPage, setAuditPage] = useState(0);
  const [auditRowsPerPage, setAuditRowsPerPage] = useState(25);
  const [clearAuditOpen, setClearAuditOpen] = useState(false);
  const [clearCommandsOpen, setClearCommandsOpen] = useState(false);
 const [unsupportedActionSn, setUnsupportedActionSn] = useState(null);
 const [selectedUnsupported, setSelectedUnsupported] = useState({});
 const [unsupportedDeleteOpen, setUnsupportedDeleteOpen] = useState(false);

 const whitelistFileRef = useRef(null);

  const setTab = useCallback(
    (tab) => {
      router.replace(
        { pathname: router.pathname, query: { ...router.query, tab } },
        undefined,
        { shallow: true }
      );
    },
    [router]
  );

  const loadedRef = useRef({
    overview: false,
    policies: false,
    exceptions: false,
  });

  const fetchOverview = useCallback(async () => {
    setLoading(true);
    try {
      const [cfg, unauthorizedResp, unsupportedResp, chartResp] = await Promise.all([
        httpRequest(`${apiPrefix}/lock/config`, 'GET'),
        httpRequest(`${apiPrefix}/lock/unauthorized`, 'GET'),
        httpRequest(`${apiPrefix}/lock/unsupported`, 'GET'),
        httpRequest(`${apiPrefix}/lock/commands?page_number=0&page_size=100`, 'GET'),
      ]);

      if (cfg.status === 200 && cfg.result) setConfig(cfg.result);
      if (unauthorizedResp.status === 200) {
        setUnauthorized(Array.isArray(unauthorizedResp.result) ? unauthorizedResp.result : []);
      }
      if (unsupportedResp.status === 200) {
        setUnsupported(Array.isArray(unsupportedResp.result) ? unsupportedResp.result : []);
      }
      if (chartResp.status === 200 && chartResp.result) {
        setChartCommands(parseListResponse(chartResp.result).items);
      }
      // Do not mark exceptions loaded here — Exceptions tab should refetch on enter
      // so new unauthorized devices are not hidden after visiting Overview first.
      loadedRef.current.overview = true;
    } finally {
      setLoading(false);
    }
  }, [apiPrefix, httpRequest]);

  const fetchPolicies = useCallback(async () => {
    setLoading(true);
    try {
      const policyResp = await httpRequest(`${apiPrefix}/lock/policies`, 'GET');
      if (policyResp.status === 200) {
        setPolicies(Array.isArray(policyResp.result) ? policyResp.result : []);
      }
      loadedRef.current.policies = true;
    } finally {
      setLoading(false);
    }
  }, [apiPrefix, httpRequest]);

  const fetchExceptions = useCallback(async () => {
    setLoading(true);
    try {
      const [unauthorizedResp, unsupportedResp] = await Promise.all([
        httpRequest(`${apiPrefix}/lock/unauthorized`, 'GET'),
        httpRequest(`${apiPrefix}/lock/unsupported`, 'GET'),
      ]);
      if (unauthorizedResp.status === 200) {
        setUnauthorized(Array.isArray(unauthorizedResp.result) ? unauthorizedResp.result : []);
      }
      if (unsupportedResp.status === 200) {
        setUnsupported(Array.isArray(unsupportedResp.result) ? unsupportedResp.result : []);
      }
      loadedRef.current.exceptions = true;
    } finally {
      setLoading(false);
    }
  }, [apiPrefix, httpRequest]);

  // Full refresh used after mutations that may touch multiple tabs.
  const fetchStatic = useCallback(async () => {
    setLoading(true);
    try {
      const [cfg, policyResp, unauthorizedResp, unsupportedResp, chartResp] = await Promise.all([
        httpRequest(`${apiPrefix}/lock/config`, 'GET'),
        httpRequest(`${apiPrefix}/lock/policies`, 'GET'),
        httpRequest(`${apiPrefix}/lock/unauthorized`, 'GET'),
        httpRequest(`${apiPrefix}/lock/unsupported`, 'GET'),
        httpRequest(`${apiPrefix}/lock/commands?page_number=0&page_size=100`, 'GET'),
      ]);

      if (cfg.status === 200 && cfg.result) setConfig(cfg.result);
      if (policyResp.status === 200) {
        setPolicies(Array.isArray(policyResp.result) ? policyResp.result : []);
      }
      if (unauthorizedResp.status === 200) {
        setUnauthorized(Array.isArray(unauthorizedResp.result) ? unauthorizedResp.result : []);
      }
      if (unsupportedResp.status === 200) {
        setUnsupported(Array.isArray(unsupportedResp.result) ? unsupportedResp.result : []);
      }
      if (chartResp.status === 200 && chartResp.result) {
        setChartCommands(parseListResponse(chartResp.result).items);
      }
      loadedRef.current = { overview: true, policies: true, exceptions: true };
    } finally {
      setLoading(false);
    }
  }, [apiPrefix, httpRequest]);

  const fetchCommands = useCallback(async () => {
    const qs = new URLSearchParams({
      page_number: String(cmdPage),
      page_size: String(cmdRowsPerPage),
    });
    if (cmdSearchApplied.trim()) {
      qs.set('sn', cmdSearchApplied.trim());
    }
    const { status, result } = await httpRequest(`${apiPrefix}/lock/commands?${qs}`, 'GET');
    if (status === 200 && result) {
      const parsed = parseListResponse(result);
      setCommands(parsed.items);
      setCommandsTotal(parsed.total);
    }
  }, [apiPrefix, httpRequest, cmdPage, cmdRowsPerPage, cmdSearchApplied]);

  const fetchAudit = useCallback(async () => {
    const qs = new URLSearchParams({
      page_number: String(auditPage),
      page_size: String(auditRowsPerPage),
    });
    if (auditSearchApplied.trim()) {
      qs.set('sn', auditSearchApplied.trim());
    }
    const { status, result } = await httpRequest(`${apiPrefix}/lock/audit?${qs}`, 'GET');
    if (status === 200 && result) {
      const parsed = parseListResponse(result);
      setAuditLogs(parsed.items);
      setAuditTotal(parsed.total);
    }
  }, [apiPrefix, httpRequest, auditPage, auditRowsPerPage, auditSearchApplied]);

  const fetchData = useCallback(async () => {
    if (activeTab === 'overview') {
      await fetchOverview();
      return;
    }
    if (activeTab === 'policies') {
      await fetchPolicies();
      return;
    }
    if (activeTab === 'exceptions') {
      await fetchExceptions();
      return;
    }
    if (activeTab === 'activity') {
      await Promise.all([fetchCommands(), fetchAudit()]);
    }
  }, [activeTab, fetchOverview, fetchPolicies, fetchExceptions, fetchCommands, fetchAudit]);

  // Lazy-load per tab; skip refetch when cached for this session (Refresh forces via fetchData).
  useEffect(() => {
    if (activeTab === 'overview' && !loadedRef.current.overview) {
      fetchOverview();
    } else if (activeTab === 'policies' && !loadedRef.current.policies) {
      fetchPolicies();
    } else if (activeTab === 'exceptions' && !loadedRef.current.exceptions) {
      fetchExceptions();
    }
  }, [activeTab, fetchOverview, fetchPolicies, fetchExceptions]);

  // Activity lists are server-paginated; load only when that tab is active.
  useEffect(() => {
    if (activeTab === 'activity') {
      fetchCommands();
    }
  }, [activeTab, fetchCommands]);

  useEffect(() => {
    if (activeTab === 'activity') {
      fetchAudit();
    }
  }, [activeTab, fetchAudit]);

  const chartAgg = useMemo(() => aggregateCommandStatuses(chartCommands), [chartCommands]);

  const kpiCounts = useMemo(
    () => ({
      unauthorized: unauthorized.length,
      unsupported: unsupported.length,
      success: chartAgg.success,
      failed: chartAgg.failed,
    }),
    [unauthorized.length, unsupported.length, chartAgg.success, chartAgg.failed]
  );

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
      fetchStatic();
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
      fetchStatic();
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
        allowed_ip_range: item.reported_ip ? hostCIDR(item.reported_ip) : '',
        description: 'Whitelisted from unauthorized list',
      })),
    });
    const { status, result } = await httpRequest(
      `${apiPrefix}/lock/unauthorized/batch-whitelist`,
      'POST',
      body
    );
    if (status === 202 || status === 207) {
      setAlert({
        severity: status === 207 ? 'warning' : 'success',
        message: `Whitelisted ${result.created ?? 0} device(s).`,
      });
      setSelectedUnauthorized({});
      fetchStatic();
    }
  };

  const dismissUnauthorizedSelected = async () => {
    const sns = Object.keys(selectedUnauthorized).filter((sn) => selectedUnauthorized[sn]);
    if (sns.length === 0) {
      setAlert({ severity: 'warning', message: 'Select at least one unauthorized device.' });
      return;
    }
    const body = JSON.stringify({ sns });
    const { status, result } = await httpRequest(
      `${apiPrefix}/lock/unauthorized/batch-delete`,
      'POST',
      body
    );
    setUnauthDismissOpen(false);
    if (status === 200) {
      setSelectedUnauthorized({});
      setAlert({
        severity: 'success',
        message: `Removed ${result?.deleted ?? sns.length} device(s) from the unauthorized list.`,
      });
      await fetchExceptions();
    } else {
      setAlert({
        severity: 'error',
        message: result?.error || result?.errors?.[0] || 'Failed to remove unauthorized devices.',
      });
    }
  };

  const deletePolicy = async (sn) => {
    setDeletingSn(sn);
    try {
      const { status } = await httpRequest(
        `${apiPrefix}/lock/policies/${encodeURIComponent(sn)}`,
        'DELETE'
      );
      if (status >= 200 && status < 300) {
        setPolicies((prev) => prev.filter((p) => p.sn !== sn));
        setUnauthorized((prev) => prev.filter((u) => u.sn !== sn));
        setAlert({ severity: 'success', message: 'Policy deleted.' });
      }
    } finally {
      setDeletingSn(null);
    }
  };

  const clearCommands = async () => {
    setClearCommandsOpen(false);
    const { status, result } = await httpRequest(`${apiPrefix}/lock/commands`, 'DELETE');
    if (status >= 200 && status < 300) {
      setCommands([]);
      setCommandsTotal(0);
      setCmdPage(0);
      setChartCommands([]);
      setAlert({
        severity: 'success',
        message: `Cleared ${result?.deleted ?? 0} command record(s).`,
      });
      fetchCommands().catch(() => {});
      fetchStatic().catch(() => {});
    } else {
      setAlert({ severity: 'error', message: result?.error || 'Failed to clear command history.' });
    }
  };

  const clearAudit = async () => {
    setClearAuditOpen(false);
    const { status, result } = await httpRequest(`${apiPrefix}/lock/audit`, 'DELETE');
    if (status >= 200 && status < 300) {
      setAuditLogs([]);
      setAuditTotal(0);
      setAuditPage(0);
      setAlert({ severity: 'success', message: `Cleared ${result?.deleted ?? 0} audit record(s).` });
      fetchAudit().catch(() => {});
    } else {
      setAlert({ severity: 'error', message: result?.error || 'Failed to clear audit history.' });
    }
  };

  const filteredPolicies = useMemo(() => {
    const q = policySearch.trim().toLowerCase();
    if (!q) return policies;
    return policies.filter((p) =>
      [p.sn, p.policy_type, p.allowed_ip_range, p.description].some((v) =>
        String(v ?? '')
          .toLowerCase()
          .includes(q)
      )
    );
  }, [policies, policySearch]);

  const pagedPolicies = useMemo(() => {
    const start = policyPage * policyRowsPerPage;
    return filteredPolicies.slice(start, start + policyRowsPerPage);
  }, [filteredPolicies, policyPage, policyRowsPerPage]);

  const filteredUnauthorized = useMemo(() => {
    const q = unauthSearch.trim().toLowerCase();
    if (!q) return unauthorized;
    return unauthorized.filter((item) =>
      [
        item.sn,
        item.reported_ip,
        item.status,
        item.reason,
        formatUnauthorizedReason(item.reason),
      ].some((v) =>
        String(v ?? '')
          .toLowerCase()
          .includes(q)
      )
    );
  }, [unauthorized, unauthSearch]);

  const pagedUnauthorized = useMemo(() => {
    const start = unauthPage * unauthRowsPerPage;
    return filteredUnauthorized.slice(start, start + unauthRowsPerPage);
  }, [filteredUnauthorized, unauthPage, unauthRowsPerPage]);

  const selectedPolicyCount = Object.values(selectedPolicies).filter(Boolean).length;

  const togglePolicy = (sn) => {
    setSelectedPolicies((prev) => ({ ...prev, [sn]: !prev[sn] }));
  };

  const allPoliciesSelected =
    pagedPolicies.length > 0 && pagedPolicies.every((p) => selectedPolicies[p.sn]);

  const toggleAllPolicies = () => {
    if (allPoliciesSelected) {
      setSelectedPolicies((prev) => {
        const next = { ...prev };
        pagedPolicies.forEach((p) => {
          delete next[p.sn];
        });
        return next;
      });
    } else {
      const next = { ...selectedPolicies };
      pagedPolicies.forEach((p) => {
        next[p.sn] = true;
      });
      setSelectedPolicies(next);
    }
  };

  const doBatchDeletePolicies = async () => {
    const sns = policies.filter((p) => selectedPolicies[p.sn]).map((p) => p.sn);
    if (sns.length === 0) return;
    const body = JSON.stringify({ sns });
    const { status, result } = await httpRequest(
      `${apiPrefix}/lock/policies/batch-delete`,
      'POST',
      body
    );
    setBatchDeleteOpen(false);
    if (status >= 200 && status < 300) {
      const deletedSet = new Set(sns);
      setPolicies((prev) => prev.filter((p) => !deletedSet.has(p.sn)));
      setUnauthorized((prev) => prev.filter((u) => !deletedSet.has(u.sn)));
      setAlert({
        severity: status === 207 ? 'warning' : 'success',
        message: `Deleted ${result.deleted ?? sns.length} policy/policies${
          result.errors?.length ? `, ${result.errors.length} error(s)` : ''
        }.`,
      });
      setSelectedPolicies({});
      fetchStatic().catch(() => {});
    } else {
      setAlert({ severity: 'error', message: result?.error || 'Batch delete failed.' });
    }
  };

  const toggleUnauthorized = (sn) => {
    setSelectedUnauthorized((prev) => ({ ...prev, [sn]: !prev[sn] }));
  };

  const allUnauthorizedSelected =
    pagedUnauthorized.length > 0 &&
    pagedUnauthorized.every((item) => selectedUnauthorized[item.sn]);

  const selectedUnauthorizedCount = Object.values(selectedUnauthorized).filter(Boolean).length;

  const toggleAllUnauthorized = () => {
    if (allUnauthorizedSelected) {
      setSelectedUnauthorized((prev) => {
        const next = { ...prev };
        pagedUnauthorized.forEach((item) => {
          delete next[item.sn];
        });
        return next;
      });
    } else {
      const next = { ...selectedUnauthorized };
      pagedUnauthorized.forEach((item) => {
        next[item.sn] = true;
      });
      setSelectedUnauthorized(next);
    }
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
        fetchStatic();
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
        fetchStatic();
     }
   } finally {
     setUnsupportedActionSn(null);
   }
 };
 
 const optOutUnsupportedDevices = unsupported.filter((d) => d.opt_out);
 const selectedUnsupportedCount = Object.values(selectedUnsupported).filter(Boolean).length;
 const allUnsupportedOptOutSelected =
   optOutUnsupportedDevices.length > 0 &&
   optOutUnsupportedDevices.every((item) => selectedUnsupported[item.sn]);
 
 const toggleUnsupported = (sn) => {
   setSelectedUnsupported((prev) => {
     const next = { ...prev };
     if (next[sn]) {
       delete next[sn];
     } else {
       next[sn] = true;
     }
     return next;
   });
 };
 
 const toggleAllUnsupported = () => {
   if (allUnsupportedOptOutSelected) {
     const next = { ...selectedUnsupported };
     optOutUnsupportedDevices.forEach((item) => delete next[item.sn]);
     setSelectedUnsupported(next);
   } else {
     const next = { ...selectedUnsupported };
     optOutUnsupportedDevices.forEach((item) => {
       next[item.sn] = true;
     });
     setSelectedUnsupported(next);
   }
 };
 
 const deleteUnsupportedSelected = async () => {
   const sns = Object.keys(selectedUnsupported).filter((sn) => selectedUnsupported[sn]);
   if (sns.length === 0) {
     setAlert({ severity: 'warning', message: 'Select at least one opted-out device.' });
     return;
   }
   const body = JSON.stringify({ sns });
   const { status, result } = await httpRequest(
     `${apiPrefix}/lock/unsupported/batch-delete`,
     'POST',
     body
   );
   setUnsupportedDeleteOpen(false);
   if (status === 200) {
     setSelectedUnsupported({});
     setAlert({
       severity: 'success',
       message: `Removed ${result?.deleted ?? sns.length} device(s) from the unsupported list.`,
     });
     await fetchStatic();
   } else {
     setAlert({
       severity: 'error',
       message: result?.error || result?.errors?.[0] || 'Failed to remove unsupported devices.',
     });
   }
 };

 const exportCommandsPage = () => {
    downloadCsv(
      'lock-commands.csv',
      ['SN', 'Target', 'Status', 'Command ID', 'Error', 'Updated'],
      commands.map((c) => [
        c.device_sn,
        c.target_status,
        c.status,
        c.command_id,
        c.error || '',
        c.updated_at ? new Date(c.updated_at).toISOString() : '',
      ])
    );
  };

  const applyCmdSearch = () => {
    setCmdPage(0);
    setCmdSearchApplied(cmdSearch.trim());
  };

  const applyAuditSearch = () => {
    setAuditPage(0);
    setAuditSearchApplied(auditSearch.trim());
  };

  return (
    <>
      <Head>
        <title>ONT Lock</title>
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

            <Tabs
              value={activeTab}
              onChange={(_, v) => setTab(v)}
              variant="scrollable"
              scrollButtons="auto"
            >
              <Tab label="Overview" value="overview" />
              <Tab label="Policies" value="policies" />
              <Tab label="Exceptions" value="exceptions" />
              <Tab label="Activity" value="activity" />
            </Tabs>

            {activeTab === 'overview' && (
              <Stack spacing={3}>
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
                    <Typography variant="body2" color="text.secondary" sx={{ mt: 2 }}>
                      When AutoLock is on, online devices are evaluated against whitelist policies.
                      Matching WAN IP unlocks; non-matching locks and lists the device under
                    Exceptions.
                    </Typography>
                  </CardContent>
                </Card>

                <OverviewKpis counts={kpiCounts} />
                <CommandsStatusChart labels={chartAgg.labels} values={chartAgg.values} />
              </Stack>
            )}

            {activeTab === 'policies' && (
              <Stack spacing={3}>
                <Card>
                  <CardHeader title="Add Whitelist" />
                  <Divider />
                  <CardContent>
                    <Stack spacing={2}>
                      <TextField
                        label="SN"
                        value={whitelistForm.sn}
                        onChange={(event) =>
                          setWhitelistForm((prev) => ({ ...prev, sn: event.target.value }))
                        }
                        required
                        fullWidth
                      />
                      <TextField
                        label="Allowed IP Range (CIDR, supports IPv4/IPv6)"
                        value={whitelistForm.allowed_ip_range}
                        onChange={(event) =>
                          setWhitelistForm((prev) => ({
                            ...prev,
                            allowed_ip_range: event.target.value,
                          }))
                        }
                        placeholder="10.10.0.0/16 or 2001:db8::/64"
                        required
                        fullWidth
                      />
                      <TextField
                        label="Description"
                        value={whitelistForm.description}
                        onChange={(event) =>
                          setWhitelistForm((prev) => ({
                            ...prev,
                            description: event.target.value,
                          }))
                        }
                        fullWidth
                        multiline
                        minRows={2}
                      />
                      <Stack direction="row" spacing={1}>
                        <Button
                          variant="contained"
                          onClick={submitPolicy}
                          disabled={!whitelistForm.sn || !whitelistForm.allowed_ip_range}
                        >
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
                        {batchResult.result?.errors?.length
                          ? ` | Errors: ${batchResult.result.errors.length}`
                          : ''}
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
                  <Stack
                    direction={{ xs: 'column', sm: 'row' }}
                    spacing={1}
                    sx={{ px: 2, pt: 2, pb: 2 }}
                    alignItems={{ sm: 'center' }}
                  >
                    <OutlinedInput
                      size="small"
                      fullWidth
                      value={policySearch}
                      onChange={(event) => {
                        setPolicySearch(event.target.value);
                        setPolicyPage(0);
                      }}
                      placeholder="Search SN / IP / description"
                      startAdornment={
                        <InputAdornment position="start">
                          <SvgIcon color="action" fontSize="small">
                            <MagnifyingGlassIcon />
                          </SvgIcon>
                        </InputAdornment>
                      }
                    />
                    {selectedPolicyCount > 0 ? (
                      <Button
                        color="error"
                        size="small"
                        variant="outlined"
                        onClick={() => setBatchDeleteOpen(true)}
                        sx={{ flexShrink: 0 }}
                      >
                        Delete Selected ({selectedPolicyCount})
                      </Button>
                    ) : null}
                  </Stack>
                  <TableContainer sx={{ overflowX: 'auto' }}>
                    <Table sx={{ minWidth: 800 }} size="small">
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
                          <TableCell>Description</TableCell>
                          <TableCell align="right">Actions</TableCell>
                        </TableRow>
                      </TableHead>
                      <TableBody>
                        {pagedPolicies.length === 0 ? (
                          <TableRow>
                            <TableCell colSpan={6} sx={{ color: 'text.secondary' }}>
                              {policies.length === 0
                                ? 'No lock policies found.'
                                : 'No policies match your search.'}
                            </TableCell>
                          </TableRow>
                        ) : (
                          pagedPolicies.map((policy) => (
                            <TableRow key={policy.sn} hover>
                              <TableCell padding="checkbox">
                                <Checkbox
                                  checked={Boolean(selectedPolicies[policy.sn])}
                                  onChange={() => togglePolicy(policy.sn)}
                                />
                              </TableCell>
                              <TruncatedCell value={policy.sn} max={20} />
                              <TruncatedCell value={policy.policy_type} max={12} />
                              <TruncatedCell value={policy.allowed_ip_range || '-'} max={22} />
                              <TruncatedCell value={policy.description || '-'} max={36} />
                              <TableCell align="right">
                                <Button
                                  color="error"
                                  size="small"
                                  disabled={deletingSn === policy.sn}
                                  onClick={() => deletePolicy(policy.sn)}
                                >
                                  Delete
                                </Button>
                              </TableCell>
                            </TableRow>
                          ))
                        )}
                      </TableBody>
                    </Table>
                  </TableContainer>
                  <TablePagination
                    component="div"
                    count={filteredPolicies.length}
                    page={policyPage}
                    onPageChange={(_, p) => setPolicyPage(p)}
                    rowsPerPage={policyRowsPerPage}
                    onRowsPerPageChange={(e) => {
                      setPolicyRowsPerPage(parseInt(e.target.value, 10));
                      setPolicyPage(0);
                    }}
                    rowsPerPageOptions={[10, 25, 50]}
                  />
                </Card>
              </Stack>
            )}

            {activeTab === 'exceptions' && (
              <Stack spacing={3}>
                <Card>
                  <CardHeader title="Unauthorized Devices" />
                  <Divider />
                  <Stack
                    direction={{ xs: 'column', sm: 'row' }}
                    spacing={1}
                    sx={{ px: 2, pt: 2, pb: 2 }}
                    alignItems={{ sm: 'center' }}
                  >
                    <OutlinedInput
                      size="small"
                      fullWidth
                      value={unauthSearch}
                      onChange={(event) => {
                        setUnauthSearch(event.target.value);
                        setUnauthPage(0);
                      }}
                      placeholder="Search SN / IP / status / reason…"
                      startAdornment={
                        <InputAdornment position="start">
                          <SvgIcon color="action" fontSize="small">
                            <MagnifyingGlassIcon />
                          </SvgIcon>
                        </InputAdornment>
                      }
                    />
                    <Button
                      size="small"
                      variant="contained"
                      onClick={batchWhitelistFromUnauthorized}
                      disabled={selectedUnauthorizedCount === 0}
                      sx={{ flexShrink: 0 }}
                    >
                      Whitelist Selected
                      {selectedUnauthorizedCount > 0 ? ` (${selectedUnauthorizedCount})` : ''}
                    </Button>
                    <Button
                      size="small"
                      color="error"
                      variant="outlined"
                      onClick={() => setUnauthDismissOpen(true)}
                      disabled={selectedUnauthorizedCount === 0}
                      sx={{ flexShrink: 0 }}
                    >
                      Remove Selected
                      {selectedUnauthorizedCount > 0 ? ` (${selectedUnauthorizedCount})` : ''}
                    </Button>
                  </Stack>
                  <TableContainer sx={{ overflowX: 'auto' }}>
                    <Table sx={{ minWidth: 800 }} size="small">
                      <TableHead>
                        <TableRow>
                          <TableCell padding="checkbox">
                            <Checkbox
                              checked={allUnauthorizedSelected}
                              indeterminate={
                                selectedUnauthorizedCount > 0 && !allUnauthorizedSelected
                              }
                              onChange={toggleAllUnauthorized}
                              disabled={pagedUnauthorized.length === 0}
                            />
                          </TableCell>
                          <TableCell>SN</TableCell>
                          <TableCell>Reported IP</TableCell>
                          <TableCell>Status</TableCell>
                          <TableCell>Reason</TableCell>
                          <TableCell>Last Seen</TableCell>
                        </TableRow>
                      </TableHead>
                      <TableBody>
                        {pagedUnauthorized.length === 0 ? (
                          <TableRow>
                            <TableCell colSpan={6} sx={{ color: 'text.secondary' }}>No unauthorized devices.</TableCell>
                          </TableRow>
                        ) : (
                          pagedUnauthorized.map((item) => (
                            <TableRow key={item.sn} hover>
                              <TableCell padding="checkbox">
                                <Checkbox
                                  checked={Boolean(selectedUnauthorized[item.sn])}
                                  onChange={() => toggleUnauthorized(item.sn)}
                                />
                              </TableCell>
                              <TruncatedCell value={item.sn} max={22} />
                              <TruncatedCell value={item.reported_ip || '-'} max={18} />
                              <TableCell>
                                <Chip
                                  label={item.status}
                                  color={statusColor(item.status)}
                                  size="small"
                                />
                              </TableCell>
                              <TruncatedCell
                                value={formatUnauthorizedReason(item.reason)}
                                max={36}
                              />
                              <TruncatedCell
                                value={
                                  item.last_seen ? new Date(item.last_seen).toLocaleString() : '-'
                                }
                                max={22}
                              />
                            </TableRow>
                          ))
                        )}
                      </TableBody>
                    </Table>
                  </TableContainer>
                  <TablePagination
                    component="div"
                    count={filteredUnauthorized.length}
                    page={unauthPage}
                    onPageChange={(_, p) => setUnauthPage(p)}
                    rowsPerPage={unauthRowsPerPage}
                    onRowsPerPageChange={(e) => {
                      setUnauthRowsPerPage(parseInt(e.target.value, 10));
                      setUnauthPage(0);
                    }}
                    rowsPerPageOptions={[10, 25, 50]}
                  />
                </Card>

                <Card>
                 <CardHeader title="Unsupported Devices" />
                 <Divider />
                 <Stack
                   direction={{ xs: 'column', sm: 'row' }}
                   spacing={1}
                   sx={{ px: 2, pt: 2 }}
                   alignItems={{ sm: 'center' }}
                   justifyContent="flex-end"
                 >
                   <Button
                     variant="outlined"
                     color="error"
                     onClick={() => setUnsupportedDeleteOpen(true)}
                     disabled={selectedUnsupportedCount === 0}
                     sx={{ flexShrink: 0 }}
                   >
                     Remove Selected
                     {selectedUnsupportedCount > 0 ? ` (${selectedUnsupportedCount})` : ''}
                   </Button>
                 </Stack>
                 <TableContainer sx={{ overflowX: 'auto' }}>
                   <Table sx={{ minWidth: 800 }} size="small">
                     <TableHead>
                       <TableRow>
                         <TableCell padding="checkbox">
                           <Checkbox
                             checked={allUnsupportedOptOutSelected}
                             indeterminate={
                               selectedUnsupportedCount > 0 && !allUnsupportedOptOutSelected
                             }
                             onChange={toggleAllUnsupported}
                             disabled={optOutUnsupportedDevices.length === 0}
                           />
                         </TableCell>
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
                           <TableCell colSpan={7} sx={{ color: 'text.secondary' }}>No unsupported devices.</TableCell>
                         </TableRow>
                       ) : (
                         unsupported.map((item) => (
                           <TableRow key={item.sn} hover>
                             <TableCell padding="checkbox">
                               <Checkbox
                                 checked={Boolean(selectedUnsupported[item.sn])}
                                 onChange={() => toggleUnsupported(item.sn)}
                                 disabled={!item.opt_out}
                               />
                             </TableCell>
                             <TruncatedCell value={item.sn} max={22} />
                              <TruncatedCell value={item.reason || '-'} max={18} />
                              <TruncatedCell value={item.detail || '-'} max={36} />
                              <TruncatedCell
                                value={
                                  item.last_checked_at
                                    ? new Date(item.last_checked_at).toLocaleString()
                                    : '-'
                                }
                                max={22}
                              />
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
                  </TableContainer>
                </Card>
              </Stack>
            )}

            {activeTab === 'activity' && (
              <Stack spacing={3}>
                <Card>
                  <CardHeader title="Recent Commands" />
                  <Divider />
                  <Stack
                    direction={{ xs: 'column', sm: 'row' }}
                    spacing={1}
                    sx={{ px: 2, pt: 2, pb: 2 }}
                    alignItems={{ sm: 'center' }}
                  >
                    <OutlinedInput
                      size="small"
                      fullWidth
                      value={cmdSearch}
                      onChange={(event) => setCmdSearch(event.target.value)}
                      onKeyDown={(event) => {
                        if (event.key === 'Enter') applyCmdSearch();
                      }}
                      placeholder="Filter by SN (exact)"
                      startAdornment={
                        <InputAdornment position="start">
                          <SvgIcon color="action" fontSize="small">
                            <MagnifyingGlassIcon />
                          </SvgIcon>
                        </InputAdornment>
                      }
                    />
                    <Button size="small" variant="outlined" onClick={applyCmdSearch} sx={{ flexShrink: 0 }}>
                      Search
                    </Button>
                    <Button
                      size="small"
                      variant="outlined"
                      startIcon={
                        <SvgIcon fontSize="small">
                          <ArrowDownTrayIcon />
                        </SvgIcon>
                      }
                      onClick={exportCommandsPage}
                      disabled={commands.length === 0}
                      sx={{ flexShrink: 0 }}
                    >
                      Export
                    </Button>
                    {commandsTotal > 0 ? (
                      <Button
                        color="error"
                        size="small"
                        variant="outlined"
                        onClick={() => setClearCommandsOpen(true)}
                        sx={{ flexShrink: 0 }}
                      >
                        Clear
                      </Button>
                    ) : null}
                  </Stack>
                  <TableContainer sx={{ overflowX: 'auto' }}>
                    <Table sx={{ minWidth: 800 }} size="small">
                      <TableHead>
                        <TableRow>
                          <TableCell>SN</TableCell>
                          <TableCell>Target</TableCell>
                          <TableCell>Status</TableCell>
                          <TableCell>Command ID</TableCell>
                          <TableCell>Updated</TableCell>
                        </TableRow>
                      </TableHead>
                      <TableBody>
                        {commands.length === 0 ? (
                          <TableRow>
                            <TableCell colSpan={5} sx={{ color: 'text.secondary' }}>No lock commands yet.</TableCell>
                          </TableRow>
                        ) : (
                          commands.map((item) => (
                            <TableRow key={item.id || item.command_id} hover>
                              <TruncatedCell value={item.device_sn} max={20} />
                              <TableCell>
                                <Chip
                                  label={item.target_status || '—'}
                                  color={statusColor(item.target_status)}
                                  size="small"
                                />
                              </TableCell>
                              <TableCell>
                                <Chip
                                  label={item.status}
                                  color={
                                    item.status === 'success'
                                      ? 'success'
                                      : item.status === 'failed'
                                        ? 'error'
                                        : 'warning'
                                  }
                                  size="small"
                                />
                              </TableCell>
                              <TruncatedCell value={item.command_id} max={24} />
                              <TruncatedCell
                                value={
                                  item.updated_at
                                    ? new Date(item.updated_at).toLocaleString()
                                    : '-'
                                }
                                max={22}
                              />
                            </TableRow>
                          ))
                        )}
                      </TableBody>
                    </Table>
                  </TableContainer>
                  <TablePagination
                    component="div"
                    count={commandsTotal}
                    page={cmdPage}
                    onPageChange={(_, p) => setCmdPage(p)}
                    rowsPerPage={cmdRowsPerPage}
                    onRowsPerPageChange={(e) => {
                      setCmdRowsPerPage(parseInt(e.target.value, 10));
                      setCmdPage(0);
                    }}
                    rowsPerPageOptions={[10, 25, 50, 100]}
                  />
                </Card>

                <Card>
                  <CardHeader title="Audit History" />
                  <Divider />
                  <Stack
                    direction={{ xs: 'column', sm: 'row' }}
                    spacing={1}
                    sx={{ px: 2, pt: 2, pb: 2 }}
                    alignItems={{ sm: 'center' }}
                  >
                    <OutlinedInput
                      size="small"
                      fullWidth
                      value={auditSearch}
                      onChange={(event) => setAuditSearch(event.target.value)}
                      onKeyDown={(event) => {
                        if (event.key === 'Enter') applyAuditSearch();
                      }}
                      placeholder="Filter by SN (exact)"
                      startAdornment={
                        <InputAdornment position="start">
                          <SvgIcon color="action" fontSize="small">
                            <MagnifyingGlassIcon />
                          </SvgIcon>
                        </InputAdornment>
                      }
                    />
                    <Button
                      size="small"
                      variant="outlined"
                      onClick={applyAuditSearch}
                      sx={{ flexShrink: 0 }}
                    >
                      Search
                    </Button>
                    {auditTotal > 0 ? (
                      <Button
                        color="error"
                        size="small"
                        variant="outlined"
                        onClick={() => setClearAuditOpen(true)}
                        sx={{ flexShrink: 0 }}
                      >
                        Clear
                      </Button>
                    ) : null}
                  </Stack>
                  <TableContainer sx={{ overflowX: 'auto' }}>
                    <Table sx={{ minWidth: 800 }} size="small">
                      <TableHead>
                        <TableRow>
                          <TableCell>Action</TableCell>
                          <TableCell>SN</TableCell>
                          <TableCell>Status</TableCell>
                          <TableCell>Operator</TableCell>
                          <TableCell>Created</TableCell>
                        </TableRow>
                      </TableHead>
                      <TableBody>
                        {auditLogs.length === 0 ? (
                          <TableRow>
                            <TableCell colSpan={5} sx={{ color: 'text.secondary' }}>No audit logs yet.</TableCell>
                          </TableRow>
                        ) : (
                          auditLogs.map((item) => (
                            <TableRow key={item.id || `${item.sn}-${item.created_at}`} hover>
                              <TruncatedCell value={item.action} max={22} />
                              <TruncatedCell value={item.sn || '-'} max={20} />
                              <TruncatedCell value={item.status || '-'} max={12} />
                              <TruncatedCell value={item.operator_id || '-'} max={24} />
                              <TruncatedCell
                                value={
                                  item.created_at
                                    ? new Date(item.created_at).toLocaleString()
                                    : '-'
                                }
                                max={22}
                              />
                            </TableRow>
                          ))
                        )}
                      </TableBody>
                    </Table>
                  </TableContainer>
                  <TablePagination
                    component="div"
                    count={auditTotal}
                    page={auditPage}
                    onPageChange={(_, p) => setAuditPage(p)}
                    rowsPerPage={auditRowsPerPage}
                    onRowsPerPageChange={(e) => {
                      setAuditRowsPerPage(parseInt(e.target.value, 10));
                      setAuditPage(0);
                    }}
                    rowsPerPageOptions={[10, 25, 50, 100]}
                  />
                </Card>
              </Stack>
            )}
          </Stack>
        </Container>
      </Box>

      <Dialog open={batchDeleteOpen} onClose={() => setBatchDeleteOpen(false)}>
        <DialogTitle>Delete {selectedPolicyCount} Policy/Policies?</DialogTitle>
        <DialogContent>
          <DialogContentText>
            This will permanently remove the selected lock policies. Affected devices will be
            re-evaluated for lock status.
          </DialogContentText>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setBatchDeleteOpen(false)}>Cancel</Button>
          <Button color="error" variant="contained" onClick={doBatchDeletePolicies}>
            Delete
          </Button>
        </DialogActions>
      </Dialog>
      <Dialog open={unauthDismissOpen} onClose={() => setUnauthDismissOpen(false)}>
        <DialogTitle>
          Remove {selectedUnauthorizedCount} Unauthorized Device
          {selectedUnauthorizedCount === 1 ? '' : 's'}?
        </DialogTitle>
        <DialogContent>
          <DialogContentText>
            This only clears the selected entries from the Unauthorized list. No whitelist policy
            is created. If a device reconnects and is still unauthorized, it will appear here again.
          </DialogContentText>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setUnauthDismissOpen(false)}>Cancel</Button>
          <Button color="error" variant="contained" onClick={dismissUnauthorizedSelected}>
            Remove
          </Button>
        </DialogActions>
     </Dialog>
     <Dialog open={unsupportedDeleteOpen} onClose={() => setUnsupportedDeleteOpen(false)}>
       <DialogTitle>
         Remove {selectedUnsupportedCount} Unsupported Device
         {selectedUnsupportedCount === 1 ? '' : 's'}?
       </DialogTitle>
       <DialogContent>
         <DialogContentText>
           This removes the selected entries from the Unsupported list. Only devices that have been
           stopped (opted out) can be removed. If a removed device comes online again, it will be
           re-probed and re-added here if it is still unsupported.
         </DialogContentText>
       </DialogContent>
       <DialogActions>
         <Button onClick={() => setUnsupportedDeleteOpen(false)}>Cancel</Button>
         <Button color="error" variant="contained" onClick={deleteUnsupportedSelected}>
           Remove
         </Button>
       </DialogActions>
     </Dialog>
     <Dialog open={clearCommandsOpen} onClose={() => setClearCommandsOpen(false)}>
        <DialogTitle>Clear Command History?</DialogTitle>
        <DialogContent>
          <DialogContentText>
            This will permanently remove completed (success/failed) lock command attempts for this
            tenant. Pending and retry commands are kept so in-flight lock/unlock retries continue.
            The clear action itself will be logged in Audit History.
          </DialogContentText>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setClearCommandsOpen(false)}>Cancel</Button>
          <Button color="error" variant="contained" onClick={clearCommands}>
            Clear
          </Button>
        </DialogActions>
      </Dialog>
      <Dialog open={clearAuditOpen} onClose={() => setClearAuditOpen(false)}>
        <DialogTitle>Clear Audit History?</DialogTitle>
        <DialogContent>
          <DialogContentText>
            This will permanently remove all lock audit records for this tenant. The clear action
            itself will be logged.
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

Page.getLayout = (page) => <DashboardLayout>{page}</DashboardLayout>;

export default Page;
