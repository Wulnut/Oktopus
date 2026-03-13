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
import ChevronDownIcon from '@heroicons/react/24/outline/ChevronDownIcon';
import WifiIcon from '@heroicons/react/24/outline/WifiIcon';
import ServerStackIcon from '@heroicons/react/24/outline/ServerStackIcon';

const formatBytes = (bytes) => {
  if (!bytes || bytes === '0') return '0 B';
  const b = parseInt(bytes, 10);
  if (isNaN(b)) return '—';
  if (b >= 1e9) return `${(b / 1e9).toFixed(1)} GB`;
  if (b >= 1e6) return `${(b / 1e6).toFixed(1)} MB`;
  if (b >= 1e3) return `${(b / 1e3).toFixed(1)} KB`;
  return `${b} B`;
};

const StatusChip = ({ value }) => {
  if (!value) return <Chip label="Unknown" size="small" variant="outlined" />;
  const color = value === 'Up' ? 'success' : value === 'Down' ? 'error' : 'default';
  return <Chip label={value} size="small" color={color} variant="outlined" />;
};

const BoolChip = ({ value }) => {
  const isTrue = value === 'true' || value === true;
  return <Chip label={isTrue ? 'Yes' : 'No'} size="small" color={isTrue ? 'success' : 'default'} variant="outlined" />;
};

// Extract Device.IP.Interface.N. and Device.IP.Interface.N.Stats. entries
const parseIpInterfaces = (data) => {
  if (!data?.req_path_results) return [];
  const ifaces = {};
  const stats = {};
  for (const r of data.req_path_results) {
    if (!r.resolved_path_results) continue;
    for (const rr of r.resolved_path_results) {
      let m = rr.resolved_path.match(/\.IP\.Interface\.(\d+)\.$/);
      if (m && rr.result_params) { ifaces[m[1]] = rr.result_params; continue; }
      m = rr.resolved_path.match(/\.IP\.Interface\.(\d+)\.Stats\.$/);
      if (m && rr.result_params) stats[m[1]] = rr.result_params;
    }
  }
  return Object.entries(ifaces)
    .sort(([a], [b]) => parseInt(a) - parseInt(b))
    .map(([idx, params]) => ({ ...params, _idx: idx, _stats: stats[idx] || null }));
};

const isAllErrors = (data) => {
  if (!data?.req_path_results?.length) return false;
  return data.req_path_results.every(r => r.err_code != null);
};

export const DevicesNetwork = ({ sn, mtp }) => {
  const { httpRequest } = useBackendContext();

  const [wifiData, setWifiData] = useState(null);
  const [wifiLoading, setWifiLoading] = useState(false);
  const [ifaceData, setIfaceData] = useState(null);
  const [ifaceLoading, setIfaceLoading] = useState(false);

  const fetchWifi = useCallback(async () => {
    if (!sn) return;
    setWifiLoading(true);
    try {
      const { status, result } = await httpRequest(`/api/device/${sn}/${mtp}/wifi-usp`, 'GET', null, null);
      if (status === 200 && result) setWifiData(result);
    } finally {
      setWifiLoading(false);
    }
  }, [sn, mtp]);

  const fetchInterfaces = useCallback(async () => {
    if (!sn) return;
    setIfaceLoading(true);
    try {
      const { status, result } = await httpRequest(`/api/device/${sn}/${mtp}/interfaces`, 'GET', null, null);
      if (status === 200 && result) setIfaceData(result);
    } finally {
      setIfaceLoading(false);
    }
  }, [sn, mtp]);

  const handleRefresh = useCallback(async () => {
    await fetchInterfaces();
    await fetchWifi();
  }, [fetchWifi, fetchInterfaces]);

  useEffect(() => {
    handleRefresh();
  }, [handleRefresh]);

  const interfaces = ifaceData ? parseIpInterfaces(ifaceData) : [];
  const wifiUnavailable = wifiData ? isAllErrors(wifiData) : false;
  const ifacesWithStats = interfaces.filter(i => i._stats);

  return (
    <Stack spacing={2}>
      <Box display="flex" justifyContent="flex-end">
        <Button
          variant="outlined"
          size="small"
          startIcon={
            (wifiLoading || ifaceLoading)
              ? <CircularProgress size={14} />
              : <SvgIcon fontSize="small"><ArrowPathIcon /></SvgIcon>
          }
          onClick={handleRefresh}
          disabled={wifiLoading || ifaceLoading}
        >
          Refresh
        </Button>
      </Box>

      {/* IP Interfaces */}
      <Card>
        <CardHeader
          avatar={<SvgIcon><ServerStackIcon /></SvgIcon>}
          title="IP Interfaces"
          subheader={ifaceData ? `${interfaces.length} interface(s) found` : 'Not loaded'}
        />
        <Divider />
        <CardContent sx={{ p: interfaces.length > 0 ? 0 : 2 }}>
          {ifaceLoading ? (
            <Box display="flex" justifyContent="center" py={4}><CircularProgress /></Box>
          ) : !ifaceData ? (
            <Typography color="text.secondary" variant="body2" textAlign="center" py={2}>
              No interface data available. Click Refresh to fetch.
            </Typography>
          ) : interfaces.length === 0 ? (
            <Typography color="text.secondary" variant="body2" textAlign="center" py={2}>
              No IP interfaces found in response.
            </Typography>
          ) : (
            <>
              <TableContainer component={Paper} elevation={0}>
                <Table size="small">
                  <TableHead>
                    <TableRow>
                      {['Name', 'Alias', 'Type', 'Status', 'IPv4', 'IPv6', 'MTU', 'Lower Layer'].map(h => (
                        <TableCell key={h} sx={{ fontWeight: 700, fontSize: '0.8rem' }}>{h}</TableCell>
                      ))}
                    </TableRow>
                  </TableHead>
                  <TableBody>
                    {interfaces.map((iface) => (
                      <TableRow key={iface._idx} hover>
                        <TableCell sx={{ fontWeight: 600, fontSize: '0.82rem' }}>{iface.Name || '—'}</TableCell>
                        <TableCell sx={{ fontSize: '0.82rem' }}>{iface.Alias || '—'}</TableCell>
                        <TableCell sx={{ fontSize: '0.82rem' }}>{iface.Type || '—'}</TableCell>
                        <TableCell><StatusChip value={iface.Status} /></TableCell>
                        <TableCell><BoolChip value={iface.IPv4Enable} /></TableCell>
                        <TableCell><BoolChip value={iface.IPv6Enable} /></TableCell>
                        <TableCell sx={{ fontSize: '0.82rem' }}>{iface.MaxMTUSize || '—'}</TableCell>
                        <TableCell sx={{ fontSize: '0.75rem', color: 'text.secondary', maxWidth: 160, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                          {iface.LowerLayers || '—'}
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </TableContainer>

              {ifacesWithStats.length > 0 && (
                <Box sx={{ p: 2 }}>
                  <Accordion disableGutters elevation={0}
                    sx={{ border: '1px solid', borderColor: 'divider', '&:before': { display: 'none' } }}
                  >
                    <AccordionSummary expandIcon={<SvgIcon fontSize="small"><ChevronDownIcon /></SvgIcon>}>
                      <Typography variant="body2" fontWeight={600}>Traffic Statistics</Typography>
                    </AccordionSummary>
                    <AccordionDetails sx={{ p: 0 }}>
                      <TableContainer component={Paper} elevation={0}>
                        <Table size="small">
                          <TableHead>
                            <TableRow>
                              {['Interface', 'RX Bytes', 'TX Bytes', 'RX Packets', 'TX Packets'].map(h => (
                                <TableCell key={h} sx={{ fontWeight: 700, fontSize: '0.8rem' }}>{h}</TableCell>
                              ))}
                            </TableRow>
                          </TableHead>
                          <TableBody>
                            {ifacesWithStats.map(iface => (
                              <TableRow key={iface._idx} hover>
                                <TableCell sx={{ fontWeight: 600, fontSize: '0.82rem' }}>
                                  {iface.Name || iface.Alias || `Interface ${iface._idx}`}
                                </TableCell>
                                <TableCell sx={{ fontSize: '0.82rem' }}>{formatBytes(iface._stats.BytesReceived)}</TableCell>
                                <TableCell sx={{ fontSize: '0.82rem' }}>{formatBytes(iface._stats.BytesSent)}</TableCell>
                                <TableCell sx={{ fontSize: '0.82rem' }}>{iface._stats.PacketsReceived || '0'}</TableCell>
                                <TableCell sx={{ fontSize: '0.82rem' }}>{iface._stats.PacketsSent || '0'}</TableCell>
                              </TableRow>
                            ))}
                          </TableBody>
                        </Table>
                      </TableContainer>
                    </AccordionDetails>
                  </Accordion>
                </Box>
              )}
            </>
          )}
        </CardContent>
      </Card>

      {/* WiFi */}
      <Card>
        <CardHeader
          avatar={<SvgIcon><WifiIcon /></SvgIcon>}
          title="WiFi"
        />
        <Divider />
        <CardContent>
          {wifiLoading ? (
            <Box display="flex" justifyContent="center" py={4}><CircularProgress /></Box>
          ) : !wifiData ? (
            <Typography color="text.secondary" variant="body2" textAlign="center" py={2}>
              No WiFi data. Click Refresh to fetch.
            </Typography>
          ) : wifiUnavailable ? (
            <Typography color="text.secondary" variant="body2" textAlign="center" py={2}>
              WiFi information is not available on this device.
            </Typography>
          ) : (
            <Typography color="text.secondary" variant="body2" textAlign="center" py={2}>
              WiFi data received.
            </Typography>
          )}
        </CardContent>
      </Card>
    </Stack>
  );
};
