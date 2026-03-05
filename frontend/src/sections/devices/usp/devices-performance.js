import { useState, useEffect, useCallback, useRef } from 'react';
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
  Grid,
  FormControlLabel,
  Switch,
  Chip,
} from '@mui/material';
import { useTheme } from '@mui/material/styles';
import { useBackendContext } from 'src/contexts/backend-context';
import { Chart } from 'src/components/chart';
import ArrowPathIcon from '@heroicons/react/24/outline/ArrowPathIcon';
import CpuChipIcon from '@heroicons/react/24/solid/CpuChipIcon';
import CircleStackIcon from '@heroicons/react/24/solid/CircleStackIcon';
import ServerStackIcon from '@heroicons/react/24/outline/ServerStackIcon';

const AUTO_REFRESH_INTERVAL = 30000;

const formatBytes = (kb) => {
  if (kb == null) return 'N/A';
  const mb = kb / 1024;
  if (mb >= 1024) return `${(mb / 1024).toFixed(1)} GB`;
  return `${mb.toFixed(0)} MB`;
};

const MetricCard = ({ title, icon, value, subValue, color }) => (
  <Card>
    <CardContent>
      <Box display="flex" alignItems="center" gap={1} mb={1}>
        <SvgIcon fontSize="small" sx={{ color }}>{icon}</SvgIcon>
        <Typography variant="subtitle2" color="text.secondary">{title}</Typography>
      </Box>
      <Typography variant="h4" fontWeight={700} color={color}>
        {value}
      </Typography>
      {subValue && (
        <Typography variant="caption" color="text.secondary">{subValue}</Typography>
      )}
    </CardContent>
  </Card>
);

const buildChartOptions = (theme, title, yFormatter) => ({
  chart: {
    background: 'transparent',
    toolbar: { show: false },
    zoom: { enabled: false },
    animations: { enabled: false },
  },
  theme: {
    mode: theme.palette.mode,
  },
  stroke: {
    curve: 'smooth',
    width: 2,
  },
  xaxis: {
    type: 'datetime',
    labels: {
      datetimeUTC: false,
      format: 'HH:mm',
      style: { colors: theme.palette.text.secondary, fontSize: '11px' },
    },
    axisBorder: { show: false },
    axisTicks: { show: false },
  },
  yaxis: {
    labels: {
      formatter: yFormatter,
      style: { colors: theme.palette.text.secondary, fontSize: '11px' },
    },
  },
  grid: {
    borderColor: theme.palette.divider,
    strokeDashArray: 4,
  },
  tooltip: {
    theme: theme.palette.mode,
    x: { format: 'dd MMM HH:mm' },
    y: { formatter: yFormatter },
  },
  legend: {
    labels: { colors: theme.palette.text.primary },
  },
  title: {
    text: title,
    style: { color: theme.palette.text.primary, fontSize: '13px' },
  },
  fill: {
    type: 'gradient',
    gradient: {
      shadeIntensity: 1,
      opacityFrom: 0.4,
      opacityTo: 0,
    },
  },
});

export const DevicesPerformance = ({ sn, mtp }) => {
  const theme = useTheme();
  const { httpRequest } = useBackendContext();

  const [latestMetrics, setLatestMetrics] = useState(null);
  const [historyMetrics, setHistoryMetrics] = useState([]);
  const [loading, setLoading] = useState(false);
  const [autoRefresh, setAutoRefresh] = useState(false);
  const intervalRef = useRef(null);

  const fetchAll = useCallback(async () => {
    if (!sn) return;
    setLoading(true);
    try {
      const [shortRes, longRes] = await Promise.all([
        httpRequest(`/api/device/${sn}/metrics?since=1`, 'GET', null, null),
        httpRequest(`/api/device/${sn}/metrics?since=24`, 'GET', null, null),
      ]);

      if (shortRes.status === 200 && Array.isArray(shortRes.result) && shortRes.result.length > 0) {
        const sorted = [...shortRes.result].sort((a, b) => new Date(b.timestamp) - new Date(a.timestamp));
        setLatestMetrics(sorted[0]);
      }

      if (longRes.status === 200 && Array.isArray(longRes.result)) {
        setHistoryMetrics(longRes.result);
      }
    } finally {
      setLoading(false);
    }
  }, [sn, httpRequest]);

  useEffect(() => {
    fetchAll();
  }, [fetchAll]);

  useEffect(() => {
    if (autoRefresh) {
      intervalRef.current = setInterval(fetchAll, AUTO_REFRESH_INTERVAL);
    } else {
      clearInterval(intervalRef.current);
    }
    return () => clearInterval(intervalRef.current);
  }, [autoRefresh, fetchAll]);

  // Derived current values
  const cpuPercent = latestMetrics?.cpu_usage != null ? `${latestMetrics.cpu_usage.toFixed(1)}%` : 'N/A';
  const memFreeKb = latestMetrics?.mem_free;
  const memTotalKb = latestMetrics?.mem_total;
  const ramUsedKb = memTotalKb != null && memFreeKb != null ? memTotalKb - memFreeKb : null;
  const ramPercent = ramUsedKb != null && memTotalKb ? ((ramUsedKb / memTotalKb) * 100).toFixed(1) : null;
  const ramDisplay = ramUsedKb != null ? formatBytes(ramUsedKb) : 'N/A';
  const ramSub = memTotalKb ? `of ${formatBytes(memTotalKb)} total${ramPercent ? ` (${ramPercent}%)` : ''}` : null;

  // Build chart series
  const sortedHistory = [...historyMetrics].sort((a, b) => new Date(a.timestamp) - new Date(b.timestamp));

  const cpuSeries = [{
    name: 'CPU %',
    data: sortedHistory
      .filter(m => m.cpu_usage != null)
      .map(m => ({ x: new Date(m.timestamp).getTime(), y: parseFloat(m.cpu_usage.toFixed(2)) })),
  }];

  const ramSeries = [{
    name: 'RAM Used (MB)',
    data: sortedHistory
      .filter(m => m.mem_total != null && m.mem_free != null)
      .map(m => ({
        x: new Date(m.timestamp).getTime(),
        y: parseFloat(((m.mem_total - m.mem_free) / 1024).toFixed(1)),
      })),
  }];

  const cpuOptions = buildChartOptions(theme, 'CPU Usage (last 24h)', v => `${v}%`);
  const ramOptions = buildChartOptions(theme, 'RAM Usage (last 24h)', v => `${v} MB`);
  cpuOptions.colors = [theme.palette.warning.main];
  ramOptions.colors = [theme.palette.info.main];

  return (
    <Stack spacing={2}>
      <Box display="flex" alignItems="center" justifyContent="space-between" flexWrap="wrap" gap={1}>
        <Box display="flex" alignItems="center" gap={1}>
          {loading && <CircularProgress size={18} />}
          {latestMetrics && (
            <Chip
              size="small"
              label={`Last updated: ${new Date(latestMetrics.timestamp).toLocaleTimeString()}`}
              variant="outlined"
            />
          )}
        </Box>
        <Box display="flex" alignItems="center" gap={2}>
          <FormControlLabel
            control={
              <Switch
                size="small"
                checked={autoRefresh}
                onChange={e => setAutoRefresh(e.target.checked)}
              />
            }
            label={<Typography variant="caption">Auto-refresh (30s)</Typography>}
          />
          <Button
            variant="outlined"
            size="small"
            startIcon={<SvgIcon fontSize="small"><ArrowPathIcon /></SvgIcon>}
            onClick={fetchAll}
            disabled={loading}
          >
            Refresh
          </Button>
        </Box>
      </Box>

      {/* Current metrics row */}
      <Grid container spacing={2}>
        <Grid item xs={12} sm={4}>
          <MetricCard
            title="CPU Usage"
            icon={<CpuChipIcon />}
            value={cpuPercent}
            color={theme.palette.warning.main}
          />
        </Grid>
        <Grid item xs={12} sm={4}>
          <MetricCard
            title="RAM Used"
            icon={<CircleStackIcon />}
            value={ramDisplay}
            subValue={ramSub}
            color={theme.palette.info.main}
          />
        </Grid>
        <Grid item xs={12} sm={4}>
          <MetricCard
            title="RAM Free"
            icon={<ServerStackIcon />}
            value={memFreeKb != null ? formatBytes(memFreeKb) : 'N/A'}
            subValue={memTotalKb ? `of ${formatBytes(memTotalKb)} total` : null}
            color={theme.palette.success.main}
          />
        </Grid>
      </Grid>

      {/* Historical charts */}
      <Card>
        <CardHeader title="CPU Usage History" subheader="Last 24 hours" />
        <Divider />
        <CardContent>
          {cpuSeries[0].data.length === 0 ? (
            <Typography color="text.secondary" variant="body2" textAlign="center" py={2}>
              No historical CPU data available.
            </Typography>
          ) : (
            <Chart
              type="area"
              series={cpuSeries}
              options={cpuOptions}
              height={240}
            />
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader title="RAM Usage History" subheader="Last 24 hours" />
        <Divider />
        <CardContent>
          {ramSeries[0].data.length === 0 ? (
            <Typography color="text.secondary" variant="body2" textAlign="center" py={2}>
              No historical RAM data available.
            </Typography>
          ) : (
            <Chart
              type="area"
              series={ramSeries}
              options={ramOptions}
              height={240}
            />
          )}
        </CardContent>
      </Card>
    </Stack>
  );
};
