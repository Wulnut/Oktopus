import PropTypes from 'prop-types';
import ComputerDesktopIcon from '@heroicons/react/24/solid/ComputerDesktopIcon';
import DeviceTabletIcon from '@heroicons/react/24/solid/DeviceTabletIcon';
import PhoneIcon from '@heroicons/react/24/solid/PhoneIcon';
import {
  Box,
  Card,
  CardContent,
  CardHeader,
  Stack,
  SvgIcon,
  Typography,
  useTheme
} from '@mui/material';
import { Chart } from 'src/components/chart';

const useChartOptions = (labels,title) => {
  const theme = useTheme();
  let options =  {
    chart: {
      background: 'transparent'
    },
    dataLabels: {
      enabled: false
    },
    labels,
    legend: {
      show: false
    },
    plotOptions: {
      pie: {
        expandOnClick: false
      }
    },
    states: {
      active: {
        filter: {
          type: 'none'
        }
      },
      hover: {
        filter: {
          type: 'none'
        }
      }
    },
    stroke: {
      width: 0
    },
    theme: {
      mode: theme.palette.mode
    },
    tooltip: {
      fillSeriesColor: false
    }
  };
  if(title === "Status"){
    options.colors = [
      theme.palette.success.main,
      theme.palette.error.main,
    ]
  }else if(title === "Vendors"){
    options.colors = [
      theme.palette.graphics.dark,
      theme.palette.graphics.darkest,
      theme.palette.graphics.light,
      theme.palette.graphics.main,
      theme.palette.graphics.lightest, 
    ]
  }
  else{
    options.colors = [
      theme.palette.info.main,
      theme.palette.warning.main,
      theme.palette.graphics.lightest, 
      theme.palette.graphics.light,
    ]
  }
  return options
};

const iconMap = {
  Desktop: (
    <SvgIcon>
      <ComputerDesktopIcon />
    </SvgIcon>
  ),
  Tablet: (
    <SvgIcon>
      <DeviceTabletIcon />
    </SvgIcon>
  ),
  Phone: (
    <SvgIcon>
      <PhoneIcon />
    </SvgIcon>
  )
};

export const OverviewTraffic = (props) => {
  const { chartSeries, labels, sx, title } = props;
  const chartOptions = useChartOptions(labels,title);

  return (
    <Card sx={sx}>
      <CardContent sx={{ py: 2, '&:last-child': { pb: 2 } }}>
        <Typography variant="subtitle2" align="center" sx={{ mb: 1 }}>
          {title}
        </Typography>
        <Chart
          height={180}
          options={chartOptions}
          series={chartSeries}
          type="donut"
          width="100%"
        />
        <Stack
          alignItems="center"
          direction="row"
          justifyContent="center"
          spacing={2}
          sx={{ mt: 1 }}
        >
          {chartSeries.map((item, index) => {
            const label = labels[index];
            return (
              <Box
                key={label}
                sx={{
                  display: 'flex',
                  flexDirection: 'column',
                  alignItems: 'center',
                }}
              >
                <Typography variant="caption" color="text.secondary">
                  {label}
                </Typography>
                <Typography variant="caption" fontWeight={600}>
                  {item}%
                </Typography>
              </Box>
            );
          })}
        </Stack>
      </CardContent>
    </Card>
  );
};

OverviewTraffic.propTypes = {
  chartSeries: PropTypes.array.isRequired,
  labels: PropTypes.array.isRequired,
  sx: PropTypes.object
};
