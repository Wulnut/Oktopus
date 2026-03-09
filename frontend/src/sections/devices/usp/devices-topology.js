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
} from '@mui/material';
import { useBackendContext } from 'src/contexts/backend-context';
import ArrowPathIcon from '@heroicons/react/24/outline/ArrowPathIcon';
import WifiIcon from '@heroicons/react/24/outline/WifiIcon';
import ComputerDesktopIcon from '@heroicons/react/24/solid/ComputerDesktopIcon';
import ServerStackIcon from '@heroicons/react/24/outline/ServerStackIcon';

// Extract objects matching a regex pattern against resolved_path in USP GetResp
const parseUspObjects = (data, pattern) => {
  if (!data?.req_path_results) return [];
  const results = {};
  for (const r of data.req_path_results) {
    if (!r.resolved_path_results) continue;
    for (const rr of r.resolved_path_results) {
      const m = rr.resolved_path.match(pattern);
      if (m && rr.result_params) results[m[1]] = rr.result_params;
    }
  }
  return Object.entries(results)
    .sort(([a], [b]) => parseInt(a) - parseInt(b))
    .map(([, v]) => v);
};

const hasPathError = (data, pathFragment) => {
  if (!data?.req_path_results) return false;
  return data.req_path_results.some(r =>
    r.requested_path && r.requested_path.includes(pathFragment) && r.err_code != null
  );
};

const parseHosts = (data) => parseUspObjects(data, /Device\.Hosts\.Host\.(\d+)\.$/);
const parseEthInterfaces = (data) => parseUspObjects(data, /Device\.Ethernet\.Interface\.(\d+)\.$/);
const parseWifiClients = (data) => parseUspObjects(data, /Device\.WiFi\.AccessPoint\.\d+\.AssociatedDevice\.(\d+)\.$/);

const ActiveChip = ({ value }) => {
  const isActive = value === 'true' || value === true;
  return <Chip label={isActive ? 'Active' : 'Inactive'} size="small" color={isActive ? 'success' : 'default'} variant="outlined" />;
};

const StatusChip = ({ value }) => {
  if (!value) return <Chip label="Unknown" size="small" variant="outlined" />;
  const color = value === 'Up' ? 'success' : value === 'Down' ? 'error' : 'default';
  return <Chip label={value} size="small" color={color} variant="outlined" />;
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
      if (status === 200 && result) setData(result);
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
  const wifiError = data ? hasPathError(data, 'WiFi.AccessPoint') : false;

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
          {/* WiFi Associated Devices */}
          <Card>
            <CardHeader
              avatar={<SvgIcon><WifiIcon /></SvgIcon>}
              title="WiFi Associated Devices"
              subheader={wifiError ? 'Not available' : `${wifiClients.length} client(s) found`}
            />
            <Divider />
            <CardContent sx={{ p: wifiClients.length > 0 ? 0 : 2 }}>
              {wifiError ? (
                <Typography color="text.secondary" variant="body2" textAlign="center" py={2}>
                  WiFi information is not available on this device.
                </Typography>
              ) : wifiClients.length === 0 ? (
                <Typography color="text.secondary" variant="body2" textAlign="center" py={2}>
                  No WiFi clients found in response.
                </Typography>
              ) : (
                <TableContainer component={Paper} elevation={0}>
                  <Table size="small">
                    <TableHead>
                      <TableRow>
                        {['MAC Address', 'IP Address', 'Signal (dBm)', 'Active'].map(h => (
                          <TableCell key={h} sx={{ fontWeight: 700, fontSize: '0.8rem' }}>{h}</TableCell>
                        ))}
                      </TableRow>
                    </TableHead>
                    <TableBody>
                      {wifiClients.map((client, idx) => (
                        <TableRow key={idx} hover>
                          <TableCell sx={{ fontFamily: 'monospace', fontSize: '0.82rem' }}>{client.MACAddress || '—'}</TableCell>
                          <TableCell sx={{ fontSize: '0.82rem' }}>{client.IPAddress || '—'}</TableCell>
                          <TableCell sx={{ fontSize: '0.82rem' }}>{client.SignalStrength || '—'}</TableCell>
                          <TableCell><ActiveChip value={client.Active} /></TableCell>
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>
                </TableContainer>
              )}
            </CardContent>
          </Card>

          {/* Connected Hosts */}
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
                <TableContainer component={Paper} elevation={0}>
                  <Table size="small">
                    <TableHead>
                      <TableRow>
                        {['Hostname', 'MAC Address', 'IP Address', 'Interface', 'Active'].map(h => (
                          <TableCell key={h} sx={{ fontWeight: 700, fontSize: '0.8rem' }}>{h}</TableCell>
                        ))}
                      </TableRow>
                    </TableHead>
                    <TableBody>
                      {hosts.map((host, idx) => (
                        <TableRow key={idx} hover>
                          <TableCell sx={{ fontWeight: 600, fontSize: '0.82rem' }}>{host.HostName || '—'}</TableCell>
                          <TableCell sx={{ fontFamily: 'monospace', fontSize: '0.82rem' }}>{host.PhysAddress || '—'}</TableCell>
                          <TableCell sx={{ fontSize: '0.82rem' }}>{host.IPAddress || '—'}</TableCell>
                          <TableCell sx={{ fontSize: '0.82rem' }}>{host.InterfaceType || '—'}</TableCell>
                          <TableCell><ActiveChip value={host.Active} /></TableCell>
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>
                </TableContainer>
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
                <TableContainer component={Paper} elevation={0}>
                  <Table size="small">
                    <TableHead>
                      <TableRow>
                        {['Name', 'Alias', 'Status', 'MAC Address', 'Duplex', 'Bitrate'].map(h => (
                          <TableCell key={h} sx={{ fontWeight: 700, fontSize: '0.8rem' }}>{h}</TableCell>
                        ))}
                      </TableRow>
                    </TableHead>
                    <TableBody>
                      {ethInterfaces.map((iface, idx) => (
                        <TableRow key={idx} hover>
                          <TableCell sx={{ fontWeight: 600, fontSize: '0.82rem' }}>{iface.Name || '—'}</TableCell>
                          <TableCell sx={{ fontSize: '0.82rem' }}>{iface.Alias || '—'}</TableCell>
                          <TableCell><StatusChip value={iface.Status} /></TableCell>
                          <TableCell sx={{ fontFamily: 'monospace', fontSize: '0.82rem' }}>{iface.MACAddress || '—'}</TableCell>
                          <TableCell sx={{ fontSize: '0.82rem' }}>{iface.CurrentDuplexMode || iface.DuplexMode || '—'}</TableCell>
                          <TableCell sx={{ fontSize: '0.82rem' }}>
                            {iface.CurrentBitRate && iface.CurrentBitRate !== '0'
                              ? `${iface.CurrentBitRate} Mbps`
                              : iface.MaxBitRate && iface.MaxBitRate !== '-1'
                                ? `${iface.MaxBitRate} Mbps max`
                                : '—'}
                          </TableCell>
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>
                </TableContainer>
              )}
            </CardContent>
          </Card>
        </>
      )}
    </Stack>
  );
};
