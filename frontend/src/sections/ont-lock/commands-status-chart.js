import PropTypes from 'prop-types';
import { Card, CardContent, CardHeader, Divider, Typography, useTheme } from '@mui/material';
import { Chart } from 'src/components/chart';

export const CommandsStatusChart = ({ labels, values }) => {
  const theme = useTheme();
  const hasData = values.some((v) => v > 0);

  const options = {
    chart: {
      background: 'transparent',
      toolbar: { show: false },
      animations: { enabled: true, speed: 600 },
    },
    colors: [theme.palette.primary.main],
    dataLabels: { enabled: true },
    fill: { opacity: 1 },
    grid: {
      borderColor: theme.palette.divider,
      strokeDashArray: 2,
    },
    plotOptions: {
      bar: {
        borderRadius: 4,
        columnWidth: '48%',
      },
    },
    theme: { mode: theme.palette.mode },
    tooltip: { theme: theme.palette.mode },
    xaxis: {
      categories: labels,
      axisBorder: { show: false },
      axisTicks: { show: false },
      labels: { style: { colors: theme.palette.text.secondary } },
    },
    yaxis: {
      labels: {
        formatter: (v) => String(Math.round(v)),
        style: { colors: theme.palette.text.secondary },
      },
      min: 0,
      forceNiceScale: true,
    },
  };

  return (
    <Card>
      <CardHeader
        title="Recent command status"
        subheader="Distribution from the latest commands window"
      />
      <Divider />
      <CardContent>
        {hasData ? (
          <Chart
            height={280}
            options={options}
            series={[{ name: 'Commands', data: values }]}
            type="bar"
            width="100%"
          />
        ) : (
          <Typography color="text.secondary" sx={{ py: 6, textAlign: 'center' }}>
            No command data yet.
          </Typography>
        )}
      </CardContent>
    </Card>
  );
};

CommandsStatusChart.propTypes = {
  labels: PropTypes.arrayOf(PropTypes.string).isRequired,
  values: PropTypes.arrayOf(PropTypes.number).isRequired,
};
