import { useState, useEffect, useCallback } from 'react';
import {
  Alert,
  Box,
  Checkbox,
  Chip,
  CircularProgress,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Paper,
  Typography,
} from '@mui/material';
import { useBackendContext } from 'src/contexts/backend-context';

export const DeviceSelector = ({ vendor, model, selectedSNs, onChange }) => {
  const { httpRequest } = useBackendContext();
  const [devices, setDevices] = useState([]);
  const [loading, setLoading] = useState(false);

  const fetchDevices = useCallback(async () => {
    setLoading(true);
    try {
      const { status, result } = await httpRequest('/api/device?page_size=500', 'GET');
      if (status === 200 && result) {
        const list = result.devices || [];
        setDevices(list);
      }
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchDevices();
  }, [fetchDevices]);

  // Filter by vendor/model when provided
  const filtered = devices.filter((d) => {
    if (vendor && d.Vendor && d.Vendor.toLowerCase() !== vendor.toLowerCase()) return false;
    if (model && d.Model && d.Model.toLowerCase() !== model.toLowerCase()) return false;
    return true;
  });

  const onlineDevices = filtered.filter((d) => d.Status === 2);
  const allSelected = onlineDevices.length > 0 && onlineDevices.every((d) => selectedSNs.includes(d.SN));

  const handleToggle = (sn) => {
    if (selectedSNs.includes(sn)) {
      onChange(selectedSNs.filter((s) => s !== sn));
    } else {
      onChange([...selectedSNs, sn]);
    }
  };

  const handleSelectAll = () => {
    if (allSelected) {
      onChange([]);
    } else {
      onChange(onlineDevices.map((d) => d.SN));
    }
  };

  if (loading) {
    return (
      <Box display="flex" justifyContent="center" py={4}>
        <CircularProgress />
      </Box>
    );
  }

  return (
    <Box>
      {vendor && model && (
        <Alert severity="info" sx={{ mb: 2 }}>
          Showing {filtered.length} device(s) matching {vendor} / {model}
          {onlineDevices.length !== filtered.length &&
            ` (${onlineDevices.length} online)`}
        </Alert>
      )}

      {filtered.length === 0 ? (
        <Alert severity="warning">
          {vendor || model
            ? `No devices found matching ${vendor || '?'} / ${model || '?'}.`
            : 'No devices found.'}
        </Alert>
      ) : (
        <>
          <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
            {selectedSNs.length} device(s) selected
          </Typography>
          <TableContainer component={Paper} variant="outlined" sx={{ maxHeight: 400 }}>
            <Table size="small" stickyHeader>
              <TableHead>
                <TableRow>
                  <TableCell padding="checkbox">
                    <Checkbox
                      size="small"
                      checked={allSelected}
                      indeterminate={selectedSNs.length > 0 && !allSelected}
                      onChange={handleSelectAll}
                    />
                  </TableCell>
                  <TableCell sx={{ fontWeight: 700 }}>Serial Number</TableCell>
                  <TableCell sx={{ fontWeight: 700 }}>Vendor</TableCell>
                  <TableCell sx={{ fontWeight: 700 }}>Model</TableCell>
                  <TableCell sx={{ fontWeight: 700 }}>Status</TableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {filtered.map((d) => {
                  const isOnline = d.Status === 2;
                  const isSelected = selectedSNs.includes(d.SN);
                  return (
                    <TableRow
                      key={d.SN}
                      hover={isOnline}
                      selected={isSelected}
                      onClick={() => isOnline && handleToggle(d.SN)}
                      sx={{
                        cursor: isOnline ? 'pointer' : 'default',
                        opacity: isOnline ? 1 : 0.5,
                      }}
                    >
                      <TableCell padding="checkbox">
                        <Checkbox
                          size="small"
                          checked={isSelected}
                          disabled={!isOnline}
                          onChange={() => handleToggle(d.SN)}
                        />
                      </TableCell>
                      <TableCell sx={{ fontSize: '0.85rem', fontWeight: 500 }}>
                        {d.Alias || d.SN}
                      </TableCell>
                      <TableCell sx={{ fontSize: '0.85rem' }}>{d.Vendor || '—'}</TableCell>
                      <TableCell sx={{ fontSize: '0.85rem' }}>{d.Model || '—'}</TableCell>
                      <TableCell>
                        <Chip
                          label={isOnline ? 'Online' : 'Offline'}
                          size="small"
                          color={isOnline ? 'success' : 'default'}
                          variant="outlined"
                        />
                      </TableCell>
                    </TableRow>
                  );
                })}
              </TableBody>
            </Table>
          </TableContainer>
        </>
      )}
    </Box>
  );
};
