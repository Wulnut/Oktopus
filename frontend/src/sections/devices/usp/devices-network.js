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
  Accordion,
  AccordionSummary,
  AccordionDetails,
} from '@mui/material';
import { useBackendContext } from 'src/contexts/backend-context';
import ArrowPathIcon from '@heroicons/react/24/outline/ArrowPathIcon';
import ChevronDownIcon from '@heroicons/react/24/outline/ChevronDownIcon';
import WifiIcon from '@heroicons/react/24/outline/WifiIcon';
import ServerStackIcon from '@heroicons/react/24/outline/ServerStackIcon';

const JsonAccordion = ({ title, data, defaultExpanded }) => {
  if (!data) return null;

  const entries = typeof data === 'object' && !Array.isArray(data)
    ? Object.entries(data)
    : [['result', data]];

  return (
    <>
      {entries.map(([key, value]) => (
        <Accordion key={key} defaultExpanded={defaultExpanded} disableGutters elevation={0}
          sx={{ border: '1px solid', borderColor: 'divider', mb: 1, '&:before': { display: 'none' } }}>
          <AccordionSummary
            expandIcon={<SvgIcon fontSize="small"><ChevronDownIcon /></SvgIcon>}
          >
            <Typography variant="body2" fontWeight={600} sx={{ wordBreak: 'break-all' }}>
              {key}
            </Typography>
          </AccordionSummary>
          <AccordionDetails sx={{ p: 0 }}>
            <Box
              component="pre"
              sx={{
                m: 0,
                p: 2,
                fontSize: '0.75rem',
                overflowX: 'auto',
                bgcolor: 'background.default',
                borderTop: '1px solid',
                borderColor: 'divider',
                whiteSpace: 'pre-wrap',
                wordBreak: 'break-word',
              }}
            >
              {JSON.stringify(value, null, 2)}
            </Box>
          </AccordionDetails>
        </Accordion>
      ))}
    </>
  );
};

const SectionCard = ({ title, icon, loading, data, emptyMessage }) => (
  <Card>
    <CardHeader
      avatar={<SvgIcon>{icon}</SvgIcon>}
      title={title}
    />
    <Divider />
    <CardContent>
      {loading ? (
        <Box display="flex" justifyContent="center" py={4}>
          <CircularProgress />
        </Box>
      ) : !data ? (
        <Typography color="text.secondary" variant="body2" textAlign="center" py={2}>
          {emptyMessage}
        </Typography>
      ) : (
        <JsonAccordion data={data} defaultExpanded={false} />
      )}
    </CardContent>
  </Card>
);

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
      if (status === 200 && result) {
        setWifiData(result);
      }
    } finally {
      setWifiLoading(false);
    }
  }, [sn, mtp]);

  const fetchInterfaces = useCallback(async () => {
    if (!sn) return;
    setIfaceLoading(true);
    try {
      const { status, result } = await httpRequest(`/api/device/${sn}/${mtp}/interfaces`, 'GET', null, null);
      if (status === 200 && result) {
        setIfaceData(result);
      }
    } finally {
      setIfaceLoading(false);
    }
  }, [sn, mtp]);

  const handleRefresh = useCallback(() => {
    fetchWifi();
    fetchInterfaces();
  }, [fetchWifi, fetchInterfaces]);

  useEffect(() => {
    handleRefresh();
  }, [handleRefresh]);

  return (
    <Stack spacing={2}>
      <Box display="flex" justifyContent="flex-end">
        <Button
          variant="outlined"
          size="small"
          startIcon={<SvgIcon fontSize="small"><ArrowPathIcon /></SvgIcon>}
          onClick={handleRefresh}
          disabled={wifiLoading || ifaceLoading}
        >
          Refresh
        </Button>
      </Box>

      <SectionCard
        title="WiFi"
        icon={<WifiIcon />}
        loading={wifiLoading}
        data={wifiData}
        emptyMessage="No WiFi data available. Click Refresh to fetch."
      />

      <SectionCard
        title="IP Interfaces"
        icon={<ServerStackIcon />}
        loading={ifaceLoading}
        data={ifaceData}
        emptyMessage="No interface data available. Click Refresh to fetch."
      />
    </Stack>
  );
};
