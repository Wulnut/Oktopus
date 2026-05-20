import { useState, useEffect, useCallback } from 'react';
import {
  Card,
  CardContent,
  CardHeader,
  Button,
  Stack,
  Box,
  SvgIcon,
  CircularProgress,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Paper,
  Divider,
  Alert,
} from '@mui/material';
import { useBackendContext } from 'src/contexts/backend-context';
import ArrowPathIcon from '@heroicons/react/24/outline/ArrowPathIcon';

const INFO_FIELDS = [
  ['Manufacturer', 'Manufacturer'],
  ['ModelName', 'Model Name'],
  ['HardwareVersion', 'Hardware Version'],
  ['SoftwareVersion', 'Software Version'],
  ['SerialNumber', 'Serial Number'],
  ['ProductClass', 'Product Class'],
  ['Description', 'Description'],
];

export const DevicesInfo = ({ sn, deviceOnline, onStatusRefresh }) => {
  const { httpRequest, apiPrefix } = useBackendContext();

  const [info, setInfo] = useState(null);
  const [loading, setLoading] = useState(false);
  const [isCached, setIsCached] = useState(false);

  // Status polling is owned by the parent (cwmp/[...id].js). We only kick a
  // one-shot refresh on explicit user action (Refresh button), not on every
  // mount, to avoid duplicate /device fetches.
  const fetchInfo = useCallback(async ({ refreshStatus = false } = {}) => {
    if (!sn) return;
    if (refreshStatus) onStatusRefresh?.();
    setLoading(true);
    try {
      const { status, result } = await httpRequest(
        `${apiPrefix}/device/cwmp/${sn}/info`,
        'GET'
      );
      if (status === 200 && result) {
        setInfo(result);
        setIsCached(!!result.cached);
      }
    } catch {
      // ignore
    } finally {
      setLoading(false);
    }
  }, [sn, apiPrefix, onStatusRefresh]);

  useEffect(() => {
    fetchInfo();
  }, [sn, fetchInfo]);

  const isOffline = deviceOnline === false;

  return (
    <>
      {isOffline && (
        <Alert severity="error" variant="filled" sx={{ fontWeight: 600 }}>
          Device is Offline
        </Alert>
      )}
      {isCached && !isOffline && (
        <Alert severity="info">
          Live CWMP query unavailable; showing last-known device data from Inform or cache.
        </Alert>
      )}
      <Card>
        <CardHeader
          title="Device Information"
          subheader={`Serial: ${sn}`}
          action={
            <Button
              size="small"
              startIcon={<SvgIcon fontSize="small"><ArrowPathIcon /></SvgIcon>}
              onClick={() => fetchInfo({ refreshStatus: true })}
              disabled={loading}
            >
              Refresh
            </Button>
          }
        />
        <Divider />
        <CardContent>
          {loading ? (
            <Box sx={{ display: 'flex', justifyContent: 'center', py: 4 }}>
              <CircularProgress />
            </Box>
          ) : (
            <TableContainer component={Paper} variant="outlined">
              <Table size="small">
                <TableHead>
                  <TableRow>
                    <TableCell>Parameter</TableCell>
                    <TableCell>Value</TableCell>
                  </TableRow>
                </TableHead>
                <TableBody>
                  {INFO_FIELDS.map(([key, label]) => (
                    <TableRow key={key}>
                      <TableCell>{label}</TableCell>
                      <TableCell>{info?.[key] ?? '-'}</TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </TableContainer>
          )}
        </CardContent>
      </Card>
    </>
  );
};
