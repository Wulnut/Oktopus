import Head from 'next/head';
import React, { useEffect, useState } from 'react';
import {
  Avatar,
  Box,
  Card,
  CardContent,
  CircularProgress,
  Container,
  Grid2 as Grid,
  SvgIcon,
  Typography,
} from '@mui/material';
import { Layout as DashboardLayout } from 'src/layouts/dashboard/layout';
import { OverviewTraffic } from 'src/sections/overview/overview-traffic';
import { OverviewCpeSettings } from 'src/sections/overview/overview-cpe-settings';
import { useRouter } from 'next/router';
import { useTenant } from 'src/contexts/tenant-context';
import CpuChipIcon from '@heroicons/react/24/solid/CpuChipIcon';
import SignalIcon from '@heroicons/react/24/solid/SignalIcon';
import SignalSlashIcon from '@heroicons/react/24/solid/SignalSlashIcon';
import BuildingOfficeIcon from '@heroicons/react/24/solid/BuildingOfficeIcon';

const DevicesCard = ({ total, online, offline }) => (
  <Card sx={{ height: '100%' }}>
    <CardContent>
      <Box sx={{ display: 'flex', alignItems: 'center', gap: 2 }}>
        <Avatar sx={{ bgcolor: 'primary.main', width: 48, height: 48 }}>
          <SvgIcon fontSize="small"><CpuChipIcon /></SvgIcon>
        </Avatar>
        <Box>
          <Typography variant="overline" color="text.secondary">
            Total Devices
          </Typography>
          <Typography variant="h4">{total}</Typography>
          <Typography variant="body2" sx={{ mt: 0.5 }}>
            <Typography component="span" variant="body2" sx={{ color: 'success.main', fontWeight: 600 }}>
              {online} online
            </Typography>
            {' / '}
            <Typography component="span" variant="body2" sx={{ color: 'error.main', fontWeight: 600 }}>
              {offline} offline
            </Typography>
          </Typography>
        </Box>
      </Box>
    </CardContent>
  </Card>
);

const StatCard = ({ label, value, icon, color }) => (
  <Card sx={{ height: '100%' }}>
    <CardContent>
      <Box sx={{ display: 'flex', alignItems: 'center', gap: 2 }}>
        <Avatar sx={{ bgcolor: color || 'primary.main', width: 48, height: 48 }}>
          <SvgIcon fontSize="small">{icon}</SvgIcon>
        </Avatar>
        <Box>
          <Typography variant="overline" color="text.secondary">
            {label}
          </Typography>
          <Typography variant="h4">{value}</Typography>
        </Box>
      </Box>
    </CardContent>
  </Card>
);

const Page = () => {
  const router = useRouter();
  const { apiPrefix, tenantSlug } = useTenant();

  const [generalInfo, setGeneralInfo] = useState(null);
  const [devicesStatus, setDevicesStatus] = useState([0, 0]);
  const [devicesCount, setDevicesCount] = useState(0);
  const [onlineCount, setOnlineCount] = useState(0);
  const [offlineCount, setOfflineCount] = useState(0);
  const [vendorsTotal, setVendorsTotal] = useState(0);
  const [productClassLabels, setProductClassLabels] = useState(['-']);
  const [productClassValues, setProductClassValues] = useState(['0']);
  const [vendorLabels, setVendorLabels] = useState(['-']);
  const [vendorValues, setVendorValues] = useState([0]);

  const fetchGeneralInfo = async () => {
    var myHeaders = new Headers();
    myHeaders.append('Content-Type', 'application/json');
    myHeaders.append('Authorization', localStorage.getItem('token'));

    var requestOptions = {
      method: 'GET',
      headers: myHeaders,
      redirect: 'follow',
    };

    let result = await fetch(
      `${process.env.NEXT_PUBLIC_REST_ENDPOINT || ''}${apiPrefix}/info/general`,
      requestOptions
    );
    if (result.status === 401) {
      router.push('/auth/login');
    } else if (result.status != 200) {
      console.log('Status:', result.status);
      let content = await result.json();
      console.log('Message:', content);
    } else {
      let content = await result.json();
      console.log('general info result:', content);
      let totalDevices = content.StatusCount.Offline + content.StatusCount.Online;
      setDevicesCount(totalDevices);
      setOnlineCount(content.StatusCount.Online);
      setOfflineCount(content.StatusCount.Offline);

      let onlinePercentage = (content.StatusCount.Online * 100) / totalDevices;

      if (Number.isInteger(onlinePercentage)) {
        setDevicesStatus([onlinePercentage, 100 - onlinePercentage]);
      } else {
        onlinePercentage = Number(onlinePercentage.toFixed(1));
        let offlinePercentage = 100 - onlinePercentage;
        setDevicesStatus([onlinePercentage, Number(offlinePercentage.toFixed(1))]);
      }

      let prodClassLabels = [];
      let prodClassValues = [];
      let prodClassValue = 0;

      content.ProductClassCount?.map((p) => {
        if (p.productClass === '') {
          prodClassLabels.push('unknown');
        } else {
          prodClassLabels.push(p.productClass);
        }
        prodClassValue += p.count;
      });

      content.ProductClassCount?.map((p) => {
        let percentageValue = (p.count * 100) / prodClassValue;
        if (Number.isInteger(percentageValue)) {
          prodClassValues.push(percentageValue);
        } else {
          prodClassValues.push(Number(percentageValue.toFixed(1)));
        }
      });

      setProductClassLabels(prodClassLabels);
      setProductClassValues(prodClassValues);

      let vLabels = [];
      let vValues = [];
      let vValue = 0;
      content.VendorsCount?.map((p) => {
        if (p.vendor === '') {
          vLabels.push('unknown');
        } else {
          vLabels.push(p.vendor);
        }
        vValue = vValue + p.count;
      });

      content.VendorsCount?.map((p) => {
        let percentageValue = (p.count * 100) / vValue;
        if (Number.isInteger(percentageValue)) {
          vValues.push(percentageValue);
        } else {
          vValues.push(Number(percentageValue.toFixed(1)));
        }
      });

      setVendorLabels(vLabels);
      setVendorValues(vValues);
      setVendorsTotal(content.VendorsCount?.length || 0);

      setGeneralInfo(content);
    }
  };

  useEffect(() => {
    fetchGeneralInfo();
  }, []);

  return generalInfo ? (
    <>
      <Head>
        <title>Oktopus | Controller</title>
      </Head>
      <Box
        component="main"
        sx={{
          flexGrow: 1,
          py: 8,
        }}
      >
        <Container maxWidth="xl">
          <Grid container spacing={3}>
            {/* Left Column */}
            <Grid size={{ xs: 12, lg: 8 }}>
              <Grid container spacing={3}>
                {/* Row 1: Devices + Vendors */}
                <Grid size={{ xs: 12, sm: 6 }}>
                  <DevicesCard
                    total={devicesCount}
                    online={onlineCount}
                    offline={offlineCount}
                  />
                </Grid>
                <Grid size={{ xs: 12, sm: 6 }}>
                  <StatCard
                    label="Vendors"
                    value={vendorsTotal}
                    icon={<BuildingOfficeIcon />}
                    color="info.main"
                  />
                </Grid>

                {/* Row 2: Donut charts */}
                <Grid size={{ xs: 12, sm: 6, md: 4 }}>
                  <OverviewTraffic
                    chartSeries={devicesStatus}
                    labels={['Online', 'Offline']}
                    sx={{ height: '100%' }}
                    title={'Status'}
                  />
                </Grid>
                <Grid size={{ xs: 12, sm: 6, md: 4 }}>
                  <OverviewTraffic
                    chartSeries={vendorValues}
                    labels={vendorLabels}
                    sx={{ height: '100%' }}
                    title={'Vendors'}
                  />
                </Grid>
                <Grid size={{ xs: 12, sm: 6, md: 4 }}>
                  <OverviewTraffic
                    chartSeries={productClassValues}
                    labels={productClassLabels}
                    sx={{ height: '100%' }}
                    title={'Devices Type'}
                  />
                </Grid>
              </Grid>
            </Grid>

            {/* Right Column: CPE Settings */}
            <Grid size={{ xs: 12, lg: 4 }}>
              <OverviewCpeSettings
                generalInfo={generalInfo}
                tenantSlug={tenantSlug}
                sx={{ height: '100%' }}
              />
            </Grid>
          </Grid>
        </Container>
      </Box>
    </>
  ) : (
    <Box sx={{ display: 'flex', justifyContent: 'center' }}>
      <CircularProgress color="inherit" />
    </Box>
  );
};

Page.getLayout = (page) => <DashboardLayout>{page}</DashboardLayout>;

export default Page;
