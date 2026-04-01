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
  Switch,
  IconButton,
  Tooltip,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  List,
  ListItem,
  ListItemButton,
  ListItemText,
  Checkbox,
  Select,
  MenuItem,
  FormControl,
  TextField,
} from '@mui/material';
import { useBackendContext } from 'src/contexts/backend-context';
import { useAlertContext } from 'src/contexts/error-context';
import ArrowPathIcon from '@heroicons/react/24/outline/ArrowPathIcon';
import LinkIcon from '@heroicons/react/24/outline/LinkIcon';
import TrashIcon from '@heroicons/react/24/outline/TrashIcon';
import PlusIcon from '@heroicons/react/24/outline/PlusIcon';
import PencilIcon from '@heroicons/react/24/outline/PencilIcon';
import CheckIcon from '@heroicons/react/24/outline/CheckIcon';
import XMarkIcon from '@heroicons/react/24/outline/XMarkIcon';

const StatusChip = ({ value }) => {
  if (!value) return <Chip label="Unknown" size="small" variant="outlined" />;
  const color = value === 'Enabled' || value === 'Up' ? 'success' : value === 'Disabled' || value === 'Down' ? 'error' : 'default';
  return <Chip label={value} size="small" color={color} variant="outlined" />;
};

// Build a lookup: "Device.Ethernet.Interface.2" → "eth0", etc.
const buildInterfaceNameMap = (ifaceData) => {
  const map = {};
  if (!ifaceData?.req_path_results) return map;
  for (const r of ifaceData.req_path_results) {
    if (!r.resolved_path_results) continue;
    for (const rr of r.resolved_path_results) {
      if (!rr.result_params) continue;
      // Strip trailing dot for lookup key
      const path = rr.resolved_path.replace(/\.$/, '');
      const name = rr.result_params.Name || rr.result_params.Alias || rr.result_params.SSID || path;
      map[path] = name;
    }
  }
  return map;
};

// Parse Device.Bridging.Bridge.{i}. and Device.Bridging.Bridge.{i}.Port.{j}.
const parseBridges = (data) => {
  if (!data?.req_path_results) return [];
  const bridges = {};
  const ports = {};
  for (const r of data.req_path_results) {
    if (!r.resolved_path_results) continue;
    for (const rr of r.resolved_path_results) {
      if (!rr.result_params) continue;
      let m = rr.resolved_path.match(/\.Bridging\.Bridge\.(\d+)\.$/);
      if (m) { bridges[m[1]] = rr.result_params; continue; }
      m = rr.resolved_path.match(/\.Bridging\.Bridge\.(\d+)\.Port\.(\d+)\.$/);
      if (m) {
        if (!ports[m[1]]) ports[m[1]] = {};
        ports[m[1]][m[2]] = rr.result_params;
      }
    }
  }
  return Object.entries(bridges)
    .sort(([a], [b]) => parseInt(a) - parseInt(b))
    .map(([idx, params]) => ({
      ...params,
      _idx: idx,
      _ports: ports[idx]
        ? Object.entries(ports[idx])
            .sort(([a], [b]) => parseInt(a) - parseInt(b))
            .map(([pIdx, pParams]) => ({ ...pParams, _idx: pIdx }))
        : [],
    }));
};

// Parse Device.Ethernet.Link.{i}. — build map from management port path to { MACAddress, Alias }
const parseEthernetLinks = (data) => {
  const map = {};
  if (!data?.req_path_results) return map;
  for (const r of data.req_path_results) {
    if (!r.resolved_path_results) continue;
    for (const rr of r.resolved_path_results) {
      if (!rr.result_params) continue;
      const m = rr.resolved_path.match(/\.Ethernet\.Link\.(\d+)\.$/);
      if (!m) continue;
      const ll = (rr.result_params.LowerLayers || '').trim().replace(/\.$/, '');
      if (ll.includes('Bridging.Bridge')) {
        map[ll] = {
          MACAddress: rr.result_params.MACAddress || '',
          Alias: rr.result_params.Alias || '',
          _linkIdx: m[1],
        };
      }
    }
  }
  return map;
};

// Resolve a LowerLayers string to friendly names
const resolveLowerLayers = (lowerLayers, nameMap) => {
  if (!lowerLayers) return [];
  return lowerLayers.split(',')
    .map(s => s.trim().replace(/\.$/, ''))
    .filter(Boolean)
    .map(path => ({ path, name: nameMap[path] || path }));
};

// Collect all interface paths already used in bridge ports
const collectUsedInterfaces = (bridges) => {
  const used = new Set();
  for (const bridge of bridges) {
    for (const port of bridge._ports) {
      if (port.LowerLayers) {
        port.LowerLayers.split(',')
          .map(s => s.trim().replace(/\.$/, ''))
          .filter(Boolean)
          .forEach(p => used.add(p));
      }
    }
  }
  return used;
};

export const DevicesBridging = ({ sn, mtp }) => {
  const { httpRequest, apiPrefix } = useBackendContext();
  const { setAlert } = useAlertContext();

  const [bridgeData, setBridgeData] = useState(null);
  const [ifaceData, setIfaceData] = useState(null);
  const [linkData, setLinkData] = useState(null);
  const [loading, setLoading] = useState(false);
  const [actionLoading, setActionLoading] = useState(null); // "bridge_idx.port_idx" or "bridge_idx"
  const [addPortDialog, setAddPortDialog] = useState(null); // { bridgeIdx, bridgePath }
  const [selectedInterfaces, setSelectedInterfaces] = useState([]);
  const [editingMAC, setEditingMAC] = useState(null); // bridgeIdx string
  const [macValue, setMacValue] = useState('');
  const [confirmAction, setConfirmAction] = useState(null); // { title, description, onConfirm }

  const uspGet = useCallback(async (paramPaths, maxDepth = 3) => {
    const body = JSON.stringify({ param_paths: paramPaths, max_depth: maxDepth });
    const { status, result } = await httpRequest(`${apiPrefix}/device/${sn}/${mtp}/get`, 'PUT', body);
    return status === 200 ? result : null;
  }, [sn, mtp]);

  const uspSet = useCallback(async (objPath, paramSettings) => {
    const body = JSON.stringify({
      allow_partial: true,
      update_objs: [{ obj_path: objPath, param_settings: paramSettings }],
    });
    const { status, result } = await httpRequest(`${apiPrefix}/device/${sn}/${mtp}/set`, 'PUT', body);
    return status === 200 ? result : null;
  }, [sn, mtp]);

  const uspAdd = useCallback(async (objPath, paramSettings) => {
    const body = JSON.stringify({
      allow_partial: true,
      create_objs: [{ obj_path: objPath, param_settings: paramSettings || [] }],
    });
    const { status, result } = await httpRequest(`${apiPrefix}/device/${sn}/${mtp}/add`, 'PUT', body);
    return status === 200 ? result : null;
  }, [sn, mtp]);

  const uspDel = useCallback(async (objPaths) => {
    const body = JSON.stringify({
      allow_partial: true,
      obj_paths: objPaths,
    });
    const { status, result } = await httpRequest(`${apiPrefix}/device/${sn}/${mtp}/del`, 'PUT', body);
    return status === 200 ? result : null;
  }, [sn, mtp]);

  const fetchAll = useCallback(async () => {
    if (!sn) return;
    setLoading(true);
    try {
      const bridgeRes = await uspGet(["Device.Bridging.Bridge."], 4);
      const ifaceRes = await uspGet(["Device.Ethernet.Interface.", "Device.WiFi.SSID."], 1);
      const linkRes = await uspGet(["Device.Ethernet.Link."], 1);
      if (bridgeRes) setBridgeData(bridgeRes);
      if (ifaceRes) setIfaceData(ifaceRes);
      if (linkRes) setLinkData(linkRes);
    } finally {
      setLoading(false);
    }
  }, [sn, uspGet]);

  useEffect(() => {
    fetchAll();
  }, [fetchAll]);

  const nameMap = ifaceData ? buildInterfaceNameMap(ifaceData) : {};
  const bridges = bridgeData ? parseBridges(bridgeData) : [];
  const ethLinkMap = linkData ? parseEthernetLinks(linkData) : {};

  const handleToggleBridgeEnable = async (bridge) => {
    const key = `bridge_${bridge._idx}`;
    setActionLoading(key);
    try {
      const newVal = bridge.Enable === 'true' || bridge.Enable === true ? 'false' : 'true';
      await uspSet(`Device.Bridging.Bridge.${bridge._idx}.`, [
        { param: 'Enable', value: newVal, required: true },
      ]);
      await fetchAll();
    } finally {
      setActionLoading(null);
    }
  };

  const handleSaveMAC = async (linkIdx) => {
    const macRegex = /^([0-9A-Fa-f]{2}[:-]){5}([0-9A-Fa-f]{2})$/;
    if (!macRegex.test(macValue)) {
      setAlert({ severity: 'error', message: 'Invalid MAC address format. Use XX:XX:XX:XX:XX:XX.' });
      return;
    }
    const key = `mac_${linkIdx}`;
    setActionLoading(key);
    setEditingMAC(null);
    try {
      await uspSet(`Device.Ethernet.Link.${linkIdx}.`, [
        { param: 'MACAddress', value: macValue, required: true },
      ]);
      await fetchAll();
    } finally {
      setActionLoading(null);
      setMacValue('');
    }
  };

  const handleTogglePortEnable = async (bridgeIdx, port) => {
    const key = `port_${bridgeIdx}_${port._idx}`;
    setActionLoading(key);
    try {
      const newVal = port.Enable === 'true' || port.Enable === true ? 'false' : 'true';
      await uspSet(`Device.Bridging.Bridge.${bridgeIdx}.Port.${port._idx}.`, [
        { param: 'Enable', value: newVal, required: true },
      ]);
      await fetchAll();
    } finally {
      setActionLoading(null);
    }
  };

  const handleDeletePort = (bridgeIdx, portIdx) => {
    setConfirmAction({
      title: 'Delete Port',
      description: `Are you sure you want to delete Port ${portIdx} from Bridge ${bridgeIdx}? This may disrupt network connectivity.`,
      onConfirm: async () => {
        const key = `del_${bridgeIdx}_${portIdx}`;
        setActionLoading(key);
        try {
          await uspDel([`Device.Bridging.Bridge.${bridgeIdx}.Port.${portIdx}.`]);
          await fetchAll();
        } finally {
          setActionLoading(null);
        }
      },
    });
  };

  const handleMovePort = (fromBridgeIdx, port, toBridgeIdx) => {
    if (fromBridgeIdx === toBridgeIdx) return;
    setConfirmAction({
      title: 'Move Port',
      description: `Move port "${port.Name || port.Alias || port._idx}" from Bridge ${fromBridgeIdx} to Bridge ${toBridgeIdx}?`,
      onConfirm: async () => {
        const key = `move_${fromBridgeIdx}_${port._idx}`;
        setActionLoading(key);
        try {
          const lowerLayers = port.LowerLayers || '';
          const enabled = port.Enable === 'true' || port.Enable === true ? 'true' : 'false';
          const name = port.Name || '';
          const alias = port.Alias || '';
          const params = [
            { param: 'LowerLayers', value: lowerLayers, required: true },
            { param: 'Enable', value: enabled, required: true },
          ];
          if (name) params.push({ param: 'Name', value: name, required: false });
          if (alias) params.push({ param: 'Alias', value: alias, required: false });
          // Add to target bridge first, then delete from source (safer order)
          const addResult = await uspAdd(`Device.Bridging.Bridge.${toBridgeIdx}.Port.`, params);
          if (!addResult) {
            setAlert({ severity: 'error', message: 'Failed to add port to target bridge. No changes made.' });
            return;
          }
          await uspDel([`Device.Bridging.Bridge.${fromBridgeIdx}.Port.${port._idx}.`]);
          await fetchAll();
        } finally {
          setActionLoading(null);
        }
      },
    });
  };

  const handleOpenAddPort = (bridgeIdx) => {
    setSelectedInterfaces([]);
    setAddPortDialog({ bridgeIdx });
  };

  const handleAddPorts = async () => {
    if (!addPortDialog || selectedInterfaces.length === 0) return;
    const { bridgeIdx } = addPortDialog;
    const key = `add_${bridgeIdx}`;
    setActionLoading(key);
    setAddPortDialog(null);
    try {
      for (const ifacePath of selectedInterfaces) {
        await uspAdd(`Device.Bridging.Bridge.${bridgeIdx}.Port.`, [
          { param: 'LowerLayers', value: ifacePath, required: true },
          { param: 'Enable', value: 'true', required: true },
        ]);
      }
      await fetchAll();
    } finally {
      setActionLoading(null);
    }
  };

  const toggleSelectedInterface = (path) => {
    setSelectedInterfaces(prev =>
      prev.includes(path) ? prev.filter(p => p !== path) : [...prev, path]
    );
  };

  // Available interfaces for adding: all known interfaces minus those already in any bridge
  const usedInterfaces = collectUsedInterfaces(bridges);
  const availableInterfaces = Object.entries(nameMap)
    .filter(([path]) => !usedInterfaces.has(path))
    .sort(([, a], [, b]) => a.localeCompare(b));

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
          onClick={fetchAll}
          disabled={loading}
        >
          Refresh
        </Button>
      </Box>

      {loading && !bridgeData && (
        <Box display="flex" justifyContent="center" py={6}>
          <CircularProgress />
        </Box>
      )}

      {!loading && !bridgeData && (
        <Card>
          <CardContent>
            <Typography color="text.secondary" variant="body2" textAlign="center" py={2}>
              No bridging data available. Click Refresh to fetch.
            </Typography>
          </CardContent>
        </Card>
      )}

      {bridgeData && bridges.length === 0 && (
        <Card>
          <CardHeader
            avatar={<SvgIcon><LinkIcon /></SvgIcon>}
            title="Bridges"
          />
          <Divider />
          <CardContent>
            <Typography color="text.secondary" variant="body2" textAlign="center" py={2}>
              No bridges found on this device.
            </Typography>
          </CardContent>
        </Card>
      )}

      {bridges.map((bridge) => {
        const bridgeEnabled = bridge.Enable === 'true' || bridge.Enable === true;
        const bridgeKey = `bridge_${bridge._idx}`;
        const isBridgeLoading = actionLoading === bridgeKey;
        const mgmtPort = bridge._ports.find(p => p.ManagementPort === 'true' || p.ManagementPort === true);
        const mgmtPortPath = mgmtPort ? `Device.Bridging.Bridge.${bridge._idx}.Port.${mgmtPort._idx}` : '';
        const ethLink = mgmtPortPath ? ethLinkMap[mgmtPortPath] : null;

        return (
          <Card key={bridge._idx}>
            <CardHeader
              avatar={<SvgIcon><LinkIcon /></SvgIcon>}
              title={
                <Box display="flex" alignItems="center" gap={1}>
                  <Typography variant="h6">
                    {bridge.Alias || bridge.Name || `Bridge ${bridge._idx}`}
                  </Typography>
                  <StatusChip value={bridge.Status} />
                </Box>
              }
              subheader={
                <Box display="flex" alignItems="center" gap={1} component="span">
                  {`${bridge._ports.length} port(s)`}
                  {ethLink && (
                    editingMAC === bridge._idx ? (
                      <>
                        <TextField
                          size="small"
                          value={macValue}
                          onChange={(e) => setMacValue(e.target.value)}
                          onKeyDown={(e) => {
                            if (e.key === 'Enter') handleSaveMAC(ethLink._linkIdx);
                            if (e.key === 'Escape') { setEditingMAC(null); setMacValue(''); }
                          }}
                          autoFocus
                          variant="standard"
                          placeholder="00:AA:BB:CC:DD:EE"
                          inputProps={{ style: { fontSize: '0.85rem', fontFamily: 'monospace' } }}
                          sx={{ width: 160, ml: 1 }}
                        />
                        <IconButton size="small" onClick={() => handleSaveMAC(ethLink._linkIdx)} disabled={!macValue}>
                          <SvgIcon fontSize="small" sx={{ fontSize: '1rem' }}><CheckIcon /></SvgIcon>
                        </IconButton>
                        <IconButton size="small" onClick={() => { setEditingMAC(null); setMacValue(''); }}>
                          <SvgIcon fontSize="small" sx={{ fontSize: '1rem' }}><XMarkIcon /></SvgIcon>
                        </IconButton>
                      </>
                    ) : (
                      <>
                        {ethLink.MACAddress && <span style={{ marginLeft: 8, fontFamily: 'monospace' }}>{ethLink.MACAddress}</span>}
                        <Tooltip title="Change MAC address">
                          <IconButton
                            size="small"
                            onClick={() => { setEditingMAC(bridge._idx); setMacValue(ethLink.MACAddress || ''); }}
                            disabled={!!actionLoading}
                          >
                            <SvgIcon fontSize="small" sx={{ fontSize: '0.9rem' }}><PencilIcon /></SvgIcon>
                          </IconButton>
                        </Tooltip>
                      </>
                    )
                  )}
                </Box>
              }
              action={
                <Box display="flex" alignItems="center" gap={1}>
                  {isBridgeLoading && <CircularProgress size={18} />}
                  <Tooltip title={bridgeEnabled ? 'Disable bridge' : 'Enable bridge'}>
                    <Switch
                      size="small"
                      checked={bridgeEnabled}
                      onChange={() => handleToggleBridgeEnable(bridge)}
                      disabled={!!actionLoading}
                    />
                  </Tooltip>
                </Box>
              }
            />
            <Divider />
            <CardContent sx={{ p: 0 }}>
              {bridge._ports.length > 0 && (
                <TableContainer component={Paper} elevation={0}>
                  <Table size="small">
                    <TableHead>
                      <TableRow>
                        {['Port', 'Status', 'Management', 'Interfaces', 'Bridge', 'Enabled', ''].map(h => (
                          <TableCell key={h} sx={{ fontWeight: 700, fontSize: '0.8rem' }}>{h}</TableCell>
                        ))}
                      </TableRow>
                    </TableHead>
                    <TableBody>
                      {bridge._ports.map((port) => {
                        const portEnabled = port.Enable === 'true' || port.Enable === true;
                        const portKey = `port_${bridge._idx}_${port._idx}`;
                        const delKey = `del_${bridge._idx}_${port._idx}`;
                        const moveKey = `move_${bridge._idx}_${port._idx}`;
                        const isPortLoading = actionLoading === portKey || actionLoading === delKey || actionLoading === moveKey;
                        const resolved = resolveLowerLayers(port.LowerLayers, nameMap);
                        const isMgmt = port.ManagementPort === 'true' || port.ManagementPort === true;

                        return (
                          <TableRow key={port._idx} hover>
                            <TableCell sx={{ fontWeight: 600, fontSize: '0.82rem' }}>
                              {port.Name || port.Alias || `Port ${port._idx}`}
                            </TableCell>
                            <TableCell><StatusChip value={port.Status} /></TableCell>
                            <TableCell>
                              {isMgmt && <Chip label="Mgmt" size="small" color="info" variant="outlined" />}
                            </TableCell>
                            <TableCell>
                              {isMgmt ? (
                                <Typography variant="caption" color="text.secondary">—</Typography>
                              ) : (
                                <Box display="flex" gap={0.5} flexWrap="wrap">
                                  {resolved.length > 0 ? (
                                    resolved.map((ll, i) => (
                                      <Tooltip key={i} title={ll.path}>
                                        <Chip label={ll.name} size="small" variant="outlined" />
                                      </Tooltip>
                                    ))
                                  ) : (
                                    <Typography variant="caption" color="text.secondary">—</Typography>
                                  )}
                                </Box>
                              )}
                            </TableCell>
                            <TableCell>
                              {isMgmt ? (
                                <Typography variant="caption" color="text.secondary">—</Typography>
                              ) : (
                                <FormControl size="small" sx={{ minWidth: 120 }}>
                                  <Select
                                    value={bridge._idx}
                                    onChange={(e) => handleMovePort(bridge._idx, port, e.target.value)}
                                    disabled={!!actionLoading || actionLoading === `move_${bridge._idx}_${port._idx}`}
                                    variant="standard"
                                    sx={{ fontSize: '0.82rem' }}
                                  >
                                    {bridges.map((b) => (
                                      <MenuItem key={b._idx} value={b._idx}>
                                        {b.Alias || b.Name || `Bridge ${b._idx}`}
                                      </MenuItem>
                                    ))}
                                  </Select>
                                </FormControl>
                              )}
                            </TableCell>
                            <TableCell>
                              {isPortLoading ? (
                                <CircularProgress size={18} />
                              ) : (
                                <Switch
                                  size="small"
                                  checked={portEnabled}
                                  onChange={() => handleTogglePortEnable(bridge._idx, port)}
                                  disabled={!!actionLoading}
                                />
                              )}
                            </TableCell>
                            <TableCell>
                              {!isMgmt && (
                                <Tooltip title="Remove port">
                                  <IconButton
                                    size="small"
                                    color="error"
                                    onClick={() => handleDeletePort(bridge._idx, port._idx)}
                                    disabled={!!actionLoading}
                                  >
                                    <SvgIcon fontSize="small"><TrashIcon /></SvgIcon>
                                  </IconButton>
                                </Tooltip>
                              )}
                            </TableCell>
                          </TableRow>
                        );
                      })}
                    </TableBody>
                  </Table>
                </TableContainer>
              )}
              <Box sx={{ p: 1.5, display: 'flex', justifyContent: 'flex-end' }}>
                <Button
                  size="small"
                  startIcon={<SvgIcon fontSize="small"><PlusIcon /></SvgIcon>}
                  onClick={() => handleOpenAddPort(bridge._idx)}
                  disabled={!!actionLoading || availableInterfaces.length === 0}
                >
                  Add Port
                </Button>
              </Box>
            </CardContent>
          </Card>
        );
      })}

      {/* Confirmation Dialog */}
      <Dialog
        open={!!confirmAction}
        onClose={() => setConfirmAction(null)}
        maxWidth="xs"
        fullWidth
      >
        <DialogTitle>{confirmAction?.title}</DialogTitle>
        <DialogContent>
          <Typography variant="body2">{confirmAction?.description}</Typography>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setConfirmAction(null)}>Cancel</Button>
          <Button
            variant="contained"
            color="error"
            onClick={() => {
              confirmAction?.onConfirm();
              setConfirmAction(null);
            }}
          >
            Confirm
          </Button>
        </DialogActions>
      </Dialog>

      {/* Add Port Dialog */}
      <Dialog
        open={!!addPortDialog}
        onClose={() => setAddPortDialog(null)}
        maxWidth="xs"
        fullWidth
      >
        <DialogTitle>Add Interfaces to Bridge</DialogTitle>
        <DialogContent>
          {availableInterfaces.length === 0 ? (
            <Typography color="text.secondary" variant="body2" py={2}>
              All interfaces are already assigned to bridges.
            </Typography>
          ) : (
            <List dense>
              {availableInterfaces.map(([path, name]) => (
                <ListItem key={path} disablePadding>
                  <ListItemButton onClick={() => toggleSelectedInterface(path)}>
                    <Checkbox
                      edge="start"
                      checked={selectedInterfaces.includes(path)}
                      disableRipple
                      size="small"
                    />
                    <ListItemText
                      primary={name}
                      secondary={path}
                      primaryTypographyProps={{ fontSize: '0.85rem' }}
                      secondaryTypographyProps={{ fontSize: '0.75rem' }}
                    />
                  </ListItemButton>
                </ListItem>
              ))}
            </List>
          )}
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setAddPortDialog(null)}>Cancel</Button>
          <Button
            variant="contained"
            onClick={handleAddPorts}
            disabled={selectedInterfaces.length === 0}
          >
            Add ({selectedInterfaces.length})
          </Button>
        </DialogActions>
      </Dialog>
    </Stack>
  );
};
