import { useState, useEffect, useCallback } from 'react';
import Head from 'next/head';
import { Alert, Box, Chip, CircularProgress, Stack, Container, Breadcrumbs, Link } from '@mui/material';
import { Layout as DashboardLayout } from 'src/layouts/dashboard/layout';
import { useRouter } from 'next/router';
import { useBackendContext } from 'src/contexts/backend-context';
import { DevicesRPC } from 'src/sections/devices/cwmp/devices-rpc';
import { DevicesDiscovery } from 'src/sections/devices/cwmp/devices-discovery';
import { DevicesInfo } from 'src/sections/devices/cwmp/devices-info';
import { DevicesWiFi } from 'src/sections/devices/cwmp/devices-wifi';
import { DevicesHistory } from 'src/sections/devices/usp/devices-history';

const Page = () => {
    const router = useRouter();
    const { httpRequest, apiPrefix } = useBackendContext();

    const deviceID = router.query.id?.[0];
    const section = router.query.id?.[1];

    const [deviceOnline, setDeviceOnline] = useState(null);

    const refreshStatus = useCallback(async () => {
        if (!deviceID) return;
        try {
            const { status, result } = await httpRequest(
                `${apiPrefix}/device?id=${encodeURIComponent(deviceID)}`,
                'GET'
            );
            if (status === 200 && result) {
                setDeviceOnline(result.Cwmp === 2 || result.Status === 2);
            } else {
                setDeviceOnline(false);
            }
        } catch {
            setDeviceOnline(false);
        }
    }, [deviceID, apiPrefix, httpRequest]);

    useEffect(() => {
        refreshStatus();
        const interval = setInterval(refreshStatus, 15000);
        return () => clearInterval(interval);
    }, [refreshStatus]);

    const showOfflineBanner = deviceOnline === false && section !== 'info';

    const sectionHandler = () => {
        switch (section) {
            case 'msg':
                return <DevicesRPC />;
            case 'discovery':
                return <DevicesDiscovery onStatusRefresh={refreshStatus} />;
            case 'info':
                return (
                    <DevicesInfo
                        sn={deviceID}
                        deviceOnline={deviceOnline}
                        onStatusRefresh={refreshStatus}
                    />
                );
            case 'wifi':
                return <DevicesWiFi />;
            case 'history':
                return <DevicesHistory />;
            default:
                router.replace(`/devices/cwmp/${deviceID}/info`);
                return null;
        }
    };

    if (!deviceID) {
        return null;
    }

    return (
        <>
            <Head>
                <title>Oktopus | Controller</title>
            </Head>
            <Box
                component="main"
                sx={{
                    flexGrow: 1,
                    py: 2,
                }}
            >
                <Container maxWidth="lg">
                    <Stack spacing={3}>
                        <Breadcrumbs separator="›" aria-label="breadcrumb">
                            <Link underline="hover" color="inherit" href="/devices">
                                Devices
                            </Link>
                            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                                <Link underline="none" color="inherit" href={`/devices/cwmp/${deviceID}/info`}>
                                    {deviceID}
                                </Link>
                                {deviceOnline === null ? (
                                    <Chip
                                        size="small"
                                        label="Fetching"
                                        icon={<CircularProgress size={12} color="inherit" />}
                                        variant="outlined"
                                    />
                                ) : deviceOnline ? (
                                    <Chip size="small" label="Online" color="success" />
                                ) : (
                                    <Chip size="small" label="Offline" color="error" />
                                )}
                            </Box>
                        </Breadcrumbs>
                        {showOfflineBanner && (
                            <Alert severity="error" variant="filled" sx={{ fontWeight: 600 }}>
                                Device is Offline — live CWMP queries are not available.
                            </Alert>
                        )}
                        {sectionHandler()}
                    </Stack>
                </Container>
            </Box>
        </>
    );
};

Page.getLayout = (page) => (
    <DashboardLayout>
        {page}
    </DashboardLayout>
);

export default Page;
