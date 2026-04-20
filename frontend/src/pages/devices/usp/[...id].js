import { useState, useEffect, useCallback } from 'react';
import Head from 'next/head';
import { Alert, Box, Chip, CircularProgress, Stack, Container, Breadcrumbs, Link } from '@mui/material';
import { Layout as DashboardLayout } from 'src/layouts/dashboard/layout';
import { useRouter } from 'next/router';
import { useBackendContext } from 'src/contexts/backend-context';
import { DevicesRPC } from 'src/sections/devices/usp/devices-rpc';
import { DevicesDiscovery } from 'src/sections/devices/usp/devices-discovery';
import { DevicesLCM } from 'src/sections/devices/usp/devices-lcm';
import { DevicesHistory } from 'src/sections/devices/usp/devices-history';
import { DevicesInfo } from 'src/sections/devices/usp/devices-info';
import { DevicesNetwork } from 'src/sections/devices/usp/devices-network';
import { DevicesBridging } from 'src/sections/devices/usp/devices-bridging';
import { DevicesPerformance } from 'src/sections/devices/usp/devices-performance';
import { DevicesTopology } from 'src/sections/devices/usp/devices-topology';

const Page = () => {
    const router = useRouter();
    const { httpRequest, apiPrefix } = useBackendContext();

    const deviceID = router.query.id[0];
    const section = router.query.id[1];

    const [deviceOnline, setDeviceOnline] = useState(null); // null = loading, true/false

    const refreshStatus = useCallback(async () => {
        try {
            const { status, result } = await httpRequest(
                `${apiPrefix}/device?id=${encodeURIComponent(deviceID)}`,
                'GET'
            );
            if (status === 200 && result) {
                setDeviceOnline(result.Status === 2);
            } else {
                setDeviceOnline(false);
            }
        } catch {
            setDeviceOnline(false);
        }
    }, [deviceID, apiPrefix]);

    useEffect(() => {
        refreshStatus();
        const interval = setInterval(refreshStatus, 15000);
        return () => clearInterval(interval);
    }, [refreshStatus]);

    const showOfflineBanner = deviceOnline === false && section !== 'info';

    const sectionHandler = () => {
        switch(section){
            case "msg":
                return <DevicesRPC onStatusRefresh={refreshStatus} />
            case "discovery":
                return <DevicesDiscovery onStatusRefresh={refreshStatus} />
            case "lcm":
                return <DevicesLCM onStatusRefresh={refreshStatus} />
            case "history":
                return <DevicesHistory/>
            case "info":
                return <DevicesInfo sn={deviceID} mtp="any" deviceOnline={deviceOnline} onOnlineChange={setDeviceOnline} onStatusRefresh={refreshStatus} />
            case "network":
                return <DevicesNetwork sn={deviceID} mtp="any" onStatusRefresh={refreshStatus} />
            case "bridging":
                return <DevicesBridging sn={deviceID} mtp="any" onStatusRefresh={refreshStatus} />
            case "performance":
                return <DevicesPerformance sn={deviceID} mtp="any" onStatusRefresh={refreshStatus} />
            case "topology":
                return <DevicesTopology sn={deviceID} mtp="any" onStatusRefresh={refreshStatus} />
            default:
                router.replace(`/devices/usp/${deviceID}/info`)
        }
    }

    return(
    <>
        <Head>
            <title>
                Oktopus | Controller
            </title>
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
                    {[<Link underline="hover" key="1" color="inherit" href="/devices">
                        Devices
                    </Link>,
                    <Box key="2" sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                        <Link
                            underline="none"
                            color="inherit"
                            href={`/devices/${deviceID}`}
                        >
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
                    </Box>]}
                    </Breadcrumbs>
                    {showOfflineBanner && (
                        <Alert severity="error" variant="filled" sx={{ fontWeight: 600 }}>
                            Device is Offline — live data is not available.
                        </Alert>
                    )}
                {
                   sectionHandler()
                }
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
