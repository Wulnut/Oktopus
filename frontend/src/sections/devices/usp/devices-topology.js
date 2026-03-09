import { useState, useEffect, useCallback } from 'react';
import {
  Card,
  CardContent,
  CardHeader,
  Button,
  Stack,
  Box,
  SvgIcon,
  Typography,
  CircularProgress,
  Divider,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Paper,
  Chip,
  Accordion,
  AccordionSummary,
  AccordionDetails,
} from '@mui/material';
import { useBackendContext } from 'src/contexts/backend-context';
import ArrowPathIcon from '@heroicons/react/24/outline/ArrowPathIcon';
import WifiIcon from '@heroicons/react/24/outline/WifiIcon';
import ComputerDesktopIcon from '@heroicons/react/24/solid/ComputerDesktopIcon';
import ServerStackIcon from '@heroicons/react/24/outline/ServerStackIcon';
import ChevronDownIcon from '@heroicons/react/24/outline/ChevronDownIcon';

// Attempt to extract a list of objects from a deeply nested USP GET response.
// USP responses often look like:
//   { "Device.WiFi.AccessPoint.1.AssociatedDevice.1.": { "MACAddress": "...", ... }, ... }
// or nested under req_obj_results / req_param_results etc.
const extractParamObjects = (data, pathFragment) => {
  if (!data || typeof data !== 'object') return [];

  // Walk the object tree and collect entries whose key matches pathFragment
  const results = [];

  const walk = (node, prefix) => {
    if (!node || typeof node !== 'object') return;
    Object.entries(node).forEach(([key, value]) => {
      const fullKey = prefix ? `${prefix}.${key}` : key;
      if (typeof key === 'string' && key.toLowerCase().includes(pathFragment.toLowerCase())) {
        if (value && typeof value === 'object') {
          results.push({ path: key, params: value });
        }
      }
      if (value && typeof value === 'object') {
        walk(value, fullKey);
      }
    });
  };

  walk(data, '');
  return results;
};

// Try to find any list of records where each record is an object with fields
const extractAnyList = (data) => {
  if (!data || typeof data !== 'object') return null;

  // Look for the first array value, or a dict of dicts
  const trySub = (node) => {
    if (!node || typeof node !== 'object') return null;
    for (const [, value] of Object.entries(node)) {
      if (Array.isArray(value) && value.length > 0 && typeof value[0] === 'object') {
        return value;
      }
      if (typeof value === 'object' && !Array.isArray(value)) {
        // dict of dicts
        const sub = Object.values(value);
        if (sub.length > 0 && typeof sub[0] === 'object' && !Array.isArray(sub[0])) {
          return sub;
        }
        const inner = trySub(value);
        if (inner) return inner;
      }
    }
    return null;
  };
  return trySub(data);
};

const StatusChip = ({ value }) => {
  if (value === undefined || value === null) return <Typography variant="caption">—</Typography>;
  const str = String(value);
  const isTrue = str.toLowerCase() === 'true' || str === '1';
  const isFalse = str.toLowerCase() === 'false' || str === '0';
  if (isTrue) return <Chip label="Active" size="small" color="success" variant="outlined" />;
  if (isFalse) return <Chip label="Inactive" size="small" color="default" variant="outlined" />;
  return <Typography variant="body2">{str}</Typography>;
};

const ParamTable = ({ items, columns }) => {
  if (!items || items.length === 0) return null;
  return (
    <TableContainer component={Paper} elevation={0} sx={{ border: '1px solid', borderColor: 'divider' }}>
      <Table size="small">
        <TableHead>
          <TableRow>
            {columns.map(col => (
              <TableCell key={col.key} sx={{ fontWeight: 700, fontSize: '0.8rem' }}>
                {col.label}
              </TableCell>
            ))}
          </TableRow>
        </TableHead>
        <TableBody>
          {items.map((item, idx) => (
            <TableRow key={idx} hover>
              {columns.map(col => (
                <TableCell key={col.key} sx={{ fontSize: '0.82rem' }}>
                  {col.key === 'Active' || col.key === 'Enable' || col.key === 'Status'
                    ? <StatusChip value={item[col.key]} />
                    : (item[col.key] != null ? String(item[col.key]) : '—')}
                </TableCell>
              ))}
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </TableContainer>
  );
};

const RawFallback = ({ data }) => (
  <Accordion disableGutters elevation={0}
    sx={{ border: '1px solid', borderColor: 'divider', '&:before': { display: 'none' } }}>
    <AccordionSummary expandIcon={<SvgIcon fontSize="small"><ChevronDownIcon /></SvgIcon>}>
      <Typography variant="body2" fontWeight={600}>Raw Response</Typography>
    </AccordionSummary>
    <AccordionDetails sx={{ p: 0 }}>
      <Box
        component="pre"
        sx={{
          m: 0, p: 2,
          fontSize: '0.72rem',
          overflowX: 'auto',
          bgcolor: 'background.default',
          whiteSpace: 'pre-wrap',
          wordBreak: 'break-word',
          borderTop: '1px solid',
          borderColor: 'divider',
        }}
      >
        {JSON.stringify(data, null, 2)}
      </Box>
    </AccordionDetails>
  </Accordion>
);

// Normalize param objects to flat rows: [{MACAddress, IPAddress, ...}]
const normalizeParamObjects = (paramObjs) =>
  paramObjs.map(({ params }) => params);

// Given the topology result, try to find WiFi clients
const parseWifiClients = (data) => {
  const found = extractParamObjects(data, 'AssociatedDevice');
  if (found.length > 0) return normalizeParamObjects(found);
  // Fallback: look for MAC-addressed objects
  return [];
};

// Parse Hosts
const parseHosts = (data) => {
  const found = extractParamObjects(data, 'Hosts.Host');
  if (found.length > 0) return normalizeParamObjects(found);
  // Try searching by "Host." pattern
  const found2 = extractParamObjects(data, 'Host.');
  if (found2.length > 0) return normalizeParamObjects(found2);
  return [];
};

// Parse Ethernet Interfaces
const parseEthInterfaces = (data) => {
  const found = extractParamObjects(data, 'Ethernet.Interface');
  if (found.length > 0) return normalizeParamObjects(found);
  return [];
};

export const DevicesTopology = ({ sn, mtp }) => {
  const { httpRequest } = useBackendContext();

  const [data, setData] = useState(null);
  const [loading, setLoading] = useState(false);

  const fetchTopology = useCallback(async () => {
    if (!sn) return;
    setLoading(true);
    try {
      const { status, result } = await httpRequest(`/api/device/${sn}/${mtp}/topology`, 'GET', null, null);
      if (status === 200 && result) {
        setData(result);
      }
    } finally {
      setLoading(false);
    }
  }, [sn, mtp]);

  useEffect(() => {
    fetchTopology();
  }, [fetchTopology]);

  const wifiClients = data ? parseWifiClients(data) : [];
  const hosts = data ? parseHosts(data) : [];
  const ethInterfaces = data ? parseEthInterfaces(data) : [];

  const hasStructuredData = wifiClients.length > 0 || hosts.length > 0 || ethInterfaces.length > 0;

  return (
    <Stack spacing={2}>
      <Box display="flex" justifyContent="flex-end">
        <Button
          variant="outlined"
          size="small"
          startIcon={
            loading
              ? <CircularProgress size={14} />
              : <SvgIcon fontSize="small"><ArrowPathIcon /></SvgIcon>
          }
          onClick={fetchTopology}
          disabled={loading}
        >
          Synchronize
        </Button>
      </Box>

      {loading && !data && (
        <Box display="flex" justifyContent="center" py={6}>
          <CircularProgress />
        </Box>
      )}

      {!loading && !data && (
        <Card>
          <CardContent>
            <Typography color="text.secondary" variant="body2" textAlign="center" py={2}>
              No topology data available. Click Synchronize to fetch.
            </Typography>
          </CardContent>
        </Card>
      )}

      {data && (
        <>
          {/* WiFi Clients */}
          <Card>
            <CardHeader
              avatar={<SvgIcon><WifiIcon /></SvgIcon>}
              title="WiFi Associated Devices"
              subheader={`${wifiClients.length} client(s) found`}
            />
            <Divider />
            <CardContent sx={{ p: wifiClients.length > 0 ? 0 : 2 }}>
              {wifiClients.length === 0 ? (
                <Typography color="text.secondary" variant="body2" textAlign="center" py={2}>
                  No WiFi clients found in response.
                </Typography>
              ) : (
                <ParamTable
                  items={wifiClients}
                  columns={[
                    { key: 'MACAddress', label: 'MAC Address' },
                    { key: 'IPAddress', label: 'IP Address' },
                    { key: 'SignalStrength', label: 'Signal (dBm)' },
                    { key: 'Active', label: 'Active' },
                  ]}
                />
              )}
            </CardContent>
          </Card>

          {/* All Hosts */}
          <Card>
            <CardHeader
              avatar={<SvgIcon><ComputerDesktopIcon /></SvgIcon>}
              title="Connected Hosts"
              subheader={`${hosts.length} host(s) found`}
            />
            <Divider />
            <CardContent sx={{ p: hosts.length > 0 ? 0 : 2 }}>
              {hosts.length === 0 ? (
                <Typography color="text.secondary" variant="body2" textAlign="center" py={2}>
                  No hosts found in response.
                </Typography>
              ) : (
                <ParamTable
                  items={hosts}
                  columns={[
                    { key: 'IPAddress', label: 'IP Address' },
                    { key: 'MACAddress', label: 'MAC Address' },
                    { key: 'HostName', label: 'Hostname' },
                    { key: 'InterfaceType', label: 'Interface' },
                    { key: 'Active', label: 'Active' },
                  ]}
                />
              )}
            </CardContent>
          </Card>

          {/* Ethernet Interfaces */}
          <Card>
            <CardHeader
              avatar={<SvgIcon><ServerStackIcon /></SvgIcon>}
              title="Ethernet Interfaces"
              subheader={`${ethInterfaces.length} interface(s) found`}
            />
            <Divider />
            <CardContent sx={{ p: ethInterfaces.length > 0 ? 0 : 2 }}>
              {ethInterfaces.length === 0 ? (
                <Typography color="text.secondary" variant="body2" textAlign="center" py={2}>
                  No Ethernet interfaces found in response.
                </Typography>
              ) : (
                <ParamTable
                  items={ethInterfaces}
                  columns={[
                    { key: 'Name', label: 'Name' },
                    { key: 'Enable', label: 'Enabled' },
                    { key: 'Status', label: 'Status' },
                    { key: 'MACAddress', label: 'MAC Address' },
                    { key: 'MaxBitRate', label: 'Max Bitrate' },
                  ]}
                />
              )}
            </CardContent>
          </Card>

          {/* Raw fallback always shown for debugging */}
          {!hasStructuredData && <RawFallback data={data} />}
        </>
      )}
    </Stack>
  );
};
