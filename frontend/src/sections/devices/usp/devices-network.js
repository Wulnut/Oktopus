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

// Extract Device.WiFi.Radio.N. entries
const parseRadios = (data) => {
  if (!data?.req_path_results) return [];
  const radios = {};
  for (const r of data.req_path_results) {
    if (!r.resolved_path_results) continue;
    for (const rr of r.resolved_path_results) {
      const m = rr.resolved_path.match(/\.WiFi\.Radio\.(\d+)\.$/);
      if (m && rr.result_params) radios[m[1]] = rr.result_params;
    }
  }
  return Object.entries(radios)
    .sort(([a], [b]) => parseInt(a) - parseInt(b))
    .map(([idx, params]) => ({ ...params, _idx: idx }));
};

// Extract Device.WiFi.SSID.N. as a lookup map keyed by path (e.g. "Device.WiFi.SSID.1.")
const parseSsidMap = (data) => {
  if (!data?.req_path_results) return {};
  const ssids = {};
  for (const r of data.req_path_results) {
    if (!r.resolved_path_results) continue;
    for (const rr of r.resolved_path_results) {
      const m = rr.resolved_path.match(/(Device\.WiFi\.SSID\.(\d+)\.)$/);
      if (m && rr.result_params) ssids[m[1]] = rr.result_params;
    }
  }
  return ssids;
};

// Extract Device.WiFi.AccessPoint.N. + Security, with resolved SSIDReference
const parseAccessPoints = (data, ssidMap) => {
  if (!data?.req_path_results) return [];
  const aps = {};
  const apSecurity = {};
  for (const r of data.req_path_results) {
    if (!r.resolved_path_results) continue;
    for (const rr of r.resolved_path_results) {
      let m = rr.resolved_path.match(/\.WiFi\.AccessPoint\.(\d+)\.$/);
      if (m && rr.result_params) { aps[m[1]] = rr.result_params; continue; }
      m = rr.resolved_path.match(/\.WiFi\.AccessPoint\.(\d+)\.Security\.$/);
      if (m && rr.result_params) { apSecurity[m[1]] = rr.result_params; continue; }
    }
  }
  return Object.entries(aps)
    .sort(([a], [b]) => parseInt(a) - parseInt(b))
    .map(([idx, params]) => {
      const ssidRef = params.SSIDReference || '';
      const ssidKey = ssidRef.endsWith('.') ? ssidRef : ssidRef + '.';
      return {
        ...params,
        _idx: idx,
        _security: apSecurity[idx] || null,
        _ssid: ssidMap[ssidKey] || ssidMap[ssidRef] || null,
      };
    });
};

// Extract Device.WiFi.EndPoint.N. with Profile, Security, and resolved SSIDReference
const parseEndPoints = (data, ssidMap) => {
  if (!data?.req_path_results) return [];
  const eps = {};
  const profiles = {};
  const profileSecurity = {};
  const epStats = {};
  for (const r of data.req_path_results) {
    if (!r.resolved_path_results) continue;
    for (const rr of r.resolved_path_results) {
      let m = rr.resolved_path.match(/\.WiFi\.EndPoint\.(\d+)\.$/);
      if (m && rr.result_params) { eps[m[1]] = rr.result_params; continue; }
      m = rr.resolved_path.match(/\.WiFi\.EndPoint\.(\d+)\.Stats\.$/);
      if (m && rr.result_params) { epStats[m[1]] = rr.result_params; continue; }
      m = rr.resolved_path.match(/\.WiFi\.EndPoint\.(\d+)\.Profile\.(\d+)\.$/);
      if (m && rr.result_params) {
        if (!profiles[m[1]]) profiles[m[1]] = {};
        profiles[m[1]][m[2]] = rr.result_params;
        continue;
      }
      m = rr.resolved_path.match(/\.WiFi\.EndPoint\.(\d+)\.Profile\.(\d+)\.Security\.$/);
      if (m && rr.result_params) {
        if (!profileSecurity[m[1]]) profileSecurity[m[1]] = {};
        profileSecurity[m[1]][m[2]] = rr.result_params;
        continue;
      }
    }
  }
  return Object.entries(eps)
    .sort(([a], [b]) => parseInt(a) - parseInt(b))
    .map(([idx, params]) => {
      const ssidRef = params.SSIDReference || '';
      const ssidKey = ssidRef.endsWith('.') ? ssidRef : ssidRef + '.';
      return {
        ...params,
        _idx: idx,
        _stats: epStats[idx] || null,
        _ssid: ssidMap[ssidKey] || ssidMap[ssidRef] || null,
        _profiles: profiles[idx]
          ? Object.entries(profiles[idx])
              .sort(([a], [b]) => parseInt(a) - parseInt(b))
              .map(([pIdx, pParams]) => ({
                ...pParams,
                _idx: pIdx,
                _security: profileSecurity[idx]?.[pIdx] || null,
              }))
          : [],
      };
    });
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

// Extract Device.Ethernet.Interface.N. entries
const parseEthInterfaces = (data) => {
  if (!data?.req_path_results) return [];
  const ifaces = {};
  for (const r of data.req_path_results) {
    if (!r.resolved_path_results) continue;
    for (const rr of r.resolved_path_results) {
      const m = rr.resolved_path.match(/\.Ethernet\.Interface\.(\d+)\.$/);
      if (m && rr.result_params) ifaces[m[1]] = rr.result_params;
    }
  }
  return Object.entries(ifaces)
    .sort(([a], [b]) => parseInt(a) - parseInt(b))
    .map(([idx, params]) => ({ ...params, _idx: idx }));
};

const isAllErrors = (data) => {
  if (!data?.req_path_results?.length) return false;
  return data.req_path_results.every(r => r.err_code != null && r.err_code !== 0);
};

export const DevicesNetwork = ({ sn, mtp, onStatusRefresh }) => {
  const { httpRequest, apiPrefix } = useBackendContext();

  const [wifiData, setWifiData] = useState(null);
  const [wifiLoading, setWifiLoading] = useState(false);
  const [ifaceData, setIfaceData] = useState(null);
  const [ifaceLoading, setIfaceLoading] = useState(false);

  const fetchWifi = useCallback(async () => {
    if (!sn) return;
    setWifiLoading(true);
    try {
      const { status, result } = await httpRequest(`${apiPrefix}/device/${sn}/${mtp}/wifi-usp`, 'GET', null, null);
      if (status === 200 && result) setWifiData(result);
    } finally {
      setWifiLoading(false);
    }
  }, [sn, mtp]);

  const fetchInterfaces = useCallback(async () => {
    if (!sn) return;
    setIfaceLoading(true);
    try {
      const { status, result } = await httpRequest(`${apiPrefix}/device/${sn}/${mtp}/interfaces`, 'GET', null, null);
      if (status === 200 && result) setIfaceData(result);
    } finally {
      setIfaceLoading(false);
    }
  }, [sn, mtp]);

  const handleRefresh = useCallback(async () => {
    onStatusRefresh?.();
    await fetchInterfaces();
    await fetchWifi();
  }, [fetchWifi, fetchInterfaces]);

  useEffect(() => {
    handleRefresh();
  }, [handleRefresh]);

  const ethInterfaces = ifaceData ? parseEthInterfaces(ifaceData) : [];
  const interfaces = ifaceData ? parseIpInterfaces(ifaceData) : [];
  const wifiUnavailable = wifiData ? isAllErrors(wifiData) : false;
  const ifacesWithStats = interfaces.filter(i => i._stats);
  const radios = wifiData ? parseRadios(wifiData) : [];
  const ssidMap = wifiData ? parseSsidMap(wifiData) : {};
  const accessPoints = wifiData ? parseAccessPoints(wifiData, ssidMap) : [];
  const endPoints = wifiData ? parseEndPoints(wifiData, ssidMap) : [];

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

      {/* Ethernet Interfaces */}
      <Card>
        <CardHeader
          avatar={<SvgIcon><ServerStackIcon /></SvgIcon>}
          title="Ethernet Interfaces"
          subheader={ifaceData ? `${ethInterfaces.length} interface(s) found` : 'Not loaded'}
        />
        <Divider />
        <CardContent sx={{ p: ethInterfaces.length > 0 ? 0 : 2 }}>
          {ifaceLoading ? (
            <Box display="flex" justifyContent="center" py={4}><CircularProgress /></Box>
          ) : !ifaceData ? (
            <Typography color="text.secondary" variant="body2" textAlign="center" py={2}>
              No interface data available. Click Refresh to fetch.
            </Typography>
          ) : ethInterfaces.length === 0 ? (
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
                  {ethInterfaces.map((iface) => (
                    <TableRow key={iface._idx} hover>
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
      {wifiLoading ? (
        <Card>
          <CardHeader avatar={<SvgIcon><WifiIcon /></SvgIcon>} title="WiFi" />
          <Divider />
          <CardContent>
            <Box display="flex" justifyContent="center" py={4}><CircularProgress /></Box>
          </CardContent>
        </Card>
      ) : !wifiData ? (
        <Card>
          <CardHeader avatar={<SvgIcon><WifiIcon /></SvgIcon>} title="WiFi" />
          <Divider />
          <CardContent>
            <Typography color="text.secondary" variant="body2" textAlign="center" py={2}>
              No WiFi data. Click Refresh to fetch.
            </Typography>
          </CardContent>
        </Card>
      ) : wifiUnavailable ? (
        <Card>
          <CardHeader avatar={<SvgIcon><WifiIcon /></SvgIcon>} title="WiFi" />
          <Divider />
          <CardContent>
            <Typography color="text.secondary" variant="body2" textAlign="center" py={2}>
              WiFi information is not available on this device.
            </Typography>
          </CardContent>
        </Card>
      ) : (
        <>
          {/* Radios */}
          {radios.length > 0 && (
            <Card>
              <CardHeader
                avatar={<SvgIcon><WifiIcon /></SvgIcon>}
                title="WiFi Radios"
                subheader={`${radios.length} radio(s) found`}
              />
              <Divider />
              <CardContent sx={{ p: 0 }}>
                <TableContainer component={Paper} elevation={0}>
                  <Table size="small">
                    <TableHead>
                      <TableRow>
                        {['Radio', 'Status', 'Band', 'Channel', 'Bandwidth', 'Standard', 'Tx Power'].map(h => (
                          <TableCell key={h} sx={{ fontWeight: 700, fontSize: '0.8rem' }}>{h}</TableCell>
                        ))}
                      </TableRow>
                    </TableHead>
                    <TableBody>
                      {radios.map((radio) => (
                        <TableRow key={radio._idx} hover>
                          <TableCell sx={{ fontWeight: 600, fontSize: '0.82rem' }}>
                            {radio.Alias || radio.Name || `Radio ${radio._idx}`}
                          </TableCell>
                          <TableCell><StatusChip value={radio.Status} /></TableCell>
                          <TableCell sx={{ fontSize: '0.82rem' }}>{radio.OperatingFrequencyBand || radio.SupportedFrequencyBands || '—'}</TableCell>
                          <TableCell sx={{ fontSize: '0.82rem' }}>{radio.Channel || '—'}</TableCell>
                          <TableCell sx={{ fontSize: '0.82rem' }}>{radio.OperatingChannelBandwidth || '—'}</TableCell>
                          <TableCell sx={{ fontSize: '0.82rem' }}>{radio.OperatingStandards || '—'}</TableCell>
                          <TableCell sx={{ fontSize: '0.82rem' }}>{radio.TransmitPower != null ? `${radio.TransmitPower}%` : '—'}</TableCell>
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>
                </TableContainer>
              </CardContent>
            </Card>
          )}

          {/* Access Points */}
          {accessPoints.length > 0 && (
            <Card>
              <CardHeader
                avatar={<SvgIcon><WifiIcon /></SvgIcon>}
                title="Access Points"
                subheader={`${accessPoints.length} access point(s) found`}
              />
              <Divider />
              <CardContent sx={{ p: 0 }}>
                <TableContainer component={Paper} elevation={0}>
                  <Table size="small">
                    <TableHead>
                      <TableRow>
                        {['AP', 'Status', 'Security', 'Enabled', 'Clients', 'SSID', 'BSSID'].map(h => (
                          <TableCell key={h} sx={{ fontWeight: 700, fontSize: '0.8rem' }}>{h}</TableCell>
                        ))}
                      </TableRow>
                    </TableHead>
                    <TableBody>
                      {accessPoints.map((ap) => (
                        <TableRow key={ap._idx} hover>
                          <TableCell sx={{ fontWeight: 600, fontSize: '0.82rem' }}>
                            {ap.Alias || `AP ${ap._idx}`}
                          </TableCell>
                          <TableCell><StatusChip value={ap.Status} /></TableCell>
                          <TableCell sx={{ fontSize: '0.82rem' }}>
                            {ap._security?.ModeEnabled || '—'}
                          </TableCell>
                          <TableCell><BoolChip value={ap.Enable} /></TableCell>
                          <TableCell sx={{ fontSize: '0.82rem' }}>
                            {ap.AssociatedDeviceNumberOfEntries ?? '—'}
                          </TableCell>
                          <TableCell sx={{ fontSize: '0.82rem' }}>
                            {ap._ssid?.SSID || '—'}
                          </TableCell>
                          <TableCell sx={{ fontSize: '0.82rem' }}>
                            {ap._ssid?.BSSID || '—'}
                          </TableCell>
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>
                </TableContainer>
              </CardContent>
            </Card>
          )}

          {/* EndPoints (Station/Client mode) */}
          {endPoints.length > 0 && (
            <Card>
              <CardHeader
                avatar={<SvgIcon><WifiIcon /></SvgIcon>}
                title="WiFi EndPoints (Client Mode)"
                subheader={`${endPoints.length} endpoint(s) found`}
              />
              <Divider />
              <CardContent sx={{ p: 0 }}>
                <TableContainer component={Paper} elevation={0}>
                  <Table size="small">
                    <TableHead>
                      <TableRow>
                        {['EndPoint', 'Status', 'Security', 'Signal', 'Noise', 'SSID', 'BSSID'].map(h => (
                          <TableCell key={h} sx={{ fontWeight: 700, fontSize: '0.8rem' }}>{h}</TableCell>
                        ))}
                      </TableRow>
                    </TableHead>
                    <TableBody>
                      {endPoints.map((ep) => {
                        const firstProfile = ep._profiles?.[0];
                        return (
                          <TableRow key={ep._idx} hover>
                            <TableCell sx={{ fontWeight: 600, fontSize: '0.82rem' }}>
                              {ep.Alias || `EndPoint ${ep._idx}`}
                            </TableCell>
                            <TableCell><StatusChip value={ep.Status} /></TableCell>
                            <TableCell sx={{ fontSize: '0.82rem' }}>
                              {firstProfile?._security?.ModeEnabled || '—'}
                            </TableCell>
                            <TableCell sx={{ fontSize: '0.82rem' }}>
                              {ep._stats?.SignalStrength != null ? `${ep._stats.SignalStrength} dBm` : '—'}
                            </TableCell>
                            <TableCell sx={{ fontSize: '0.82rem' }}>
                              {ep._stats?.Noise != null ? `${ep._stats.Noise} dBm` : '—'}
                            </TableCell>
                            <TableCell sx={{ fontSize: '0.82rem' }}>
                              {ep._ssid?.SSID || '—'}
                            </TableCell>
                            <TableCell sx={{ fontSize: '0.82rem' }}>
                              {ep._ssid?.BSSID || '—'}
                            </TableCell>
                          </TableRow>
                        );
                      })}
                    </TableBody>
                  </Table>
                </TableContainer>
              </CardContent>
            </Card>
          )}

          {/* Fallback if no WiFi objects found */}
          {radios.length === 0 && accessPoints.length === 0 && endPoints.length === 0 && (
            <Card>
              <CardHeader avatar={<SvgIcon><WifiIcon /></SvgIcon>} title="WiFi" />
              <Divider />
              <CardContent>
                <Typography color="text.secondary" variant="body2" textAlign="center" py={2}>
                  No WiFi configuration found in device response.
                </Typography>
              </CardContent>
            </Card>
          )}
        </>
      )}
    </Stack>
  );
};
