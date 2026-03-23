import { useState, useEffect } from 'react';
import Head from 'next/head';
import { Alert, Box, Stack, Container, Breadcrumbs, Link } from '@mui/material';
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
    const { httpRequest } = useBackendContext();

    const deviceID = router.query.id[0];
    const section = router.query.id[1];

    const [deviceOnline, setDeviceOnline] = useState(null); // null = loading, true/false

    useEffect(() => {
        const fetchStatus = async () => {
            try {
                const { status, result } = await httpRequest(
                    `/api/device?id=${encodeURIComponent(deviceID)}`,
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
        };
        fetchStatus();
    }, [deviceID]);

    const showOfflineBanner = deviceOnline === false && section !== 'info';

    const sectionHandler = () => {
        switch(section){
            case "msg":
                return <DevicesRPC/>
            case "discovery":
                return <DevicesDiscovery/>
            case "lcm":
                return <DevicesLCM/>
            case "history":
                return <DevicesHistory/>
            case "info":
                return <DevicesInfo sn={deviceID} mtp="any" deviceOnline={deviceOnline} onOnlineChange={setDeviceOnline} />
            case "network":
                return <DevicesNetwork sn={deviceID} mtp="any" />
            case "bridging":
                return <DevicesBridging sn={deviceID} mtp="any" />
            case "performance":
                return <DevicesPerformance sn={deviceID} mtp="any" />
            case "topology":
                return <DevicesTopology sn={deviceID} mtp="any" />
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
                    <Link
                    underline="none"
                    key="2"
                    color="inherit"
                    href={`/devices/${deviceID}`}
                    >
                    {deviceID}
                    </Link>]}
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
