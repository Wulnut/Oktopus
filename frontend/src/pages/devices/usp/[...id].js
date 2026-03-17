import Head from 'next/head';
import { Box, Stack, Typography, Container, Unstable_Grid2 as Grid,
Tab,
Tabs,
SvgIcon,
Breadcrumbs,
Link,
CircularProgress,
Tooltip,
Button,
Menu,
MenuItem} from '@mui/material';
import { Layout as DashboardLayout } from 'src/layouts/dashboard/layout';
import { useRouter } from 'next/router';
import { DevicesRPC } from 'src/sections/devices/usp/devices-rpc';
import { DevicesDiscovery } from 'src/sections/devices/usp/devices-discovery';
import { DevicesLCM } from 'src/sections/devices/usp/devices-lcm';
import { DevicesHistory } from 'src/sections/devices/usp/devices-history';
import { DevicesInfo } from 'src/sections/devices/usp/devices-info';
import { DevicesNetwork } from 'src/sections/devices/usp/devices-network';
import { DevicesPerformance } from 'src/sections/devices/usp/devices-performance';
import { DevicesTopology } from 'src/sections/devices/usp/devices-topology';
import EnvelopeIcon from '@heroicons/react/24/outline/EnvelopeIcon';
import MagnifyingGlassIcon from '@heroicons/react/24/solid/MagnifyingGlassIcon';
import WifiIcon from '@heroicons/react/24/outline/WifiIcon';
import ServerStackIcon from '@heroicons/react/24/outline/ServerStackIcon';
import { useState } from 'react';
import SignalIcon from '@heroicons/react/24/solid/SignalIcon';
import DevicePhoneMobile from '@heroicons/react/24/solid/DevicePhoneMobileIcon';
import WrenchScrewDriverIcon from '@heroicons/react/24/outline/WrenchScrewdriverIcon';
import CommandLineIcon from '@heroicons/react/24/outline/CommandLineIcon';
import CubeTransparentIcon from '@heroicons/react/24/outline/CubeTransparentIcon';
import MapPin from '@heroicons/react/24/outline/MapPinIcon';
import ArrowTrendingUp from '@heroicons/react/24/outline/ArrowTrendingUpIcon';
import ClockIcon from '@heroicons/react/24/outline/ClockIcon';
import InformationCircleIcon from '@heroicons/react/24/outline/InformationCircleIcon';
import GlobeAltIcon from '@heroicons/react/24/outline/GlobeAltIcon';
import ChartBarIcon from '@heroicons/react/24/outline/ChartBarIcon';
import MapIcon from '@heroicons/react/24/outline/MapIcon';

const Page = () => {
    const router = useRouter()

    const deviceID = router.query.id[0]
    const section = router.query.id[1]
    
    const [loading, setLoading] = useState(true)
    const [unimplementedAnchor, setUnimplementedAnchor] = useState(null)

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
                return <DevicesInfo sn={deviceID} mtp="any" />
            case "network":
                return <DevicesNetwork sn={deviceID} mtp="any" />
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
                py: 0,
            }}
        >
            <Container maxWidth="xg" >
                <Stack spacing={3} mb={3}>
                    <Breadcrumbs separator="›" aria-label="breadcrumb"ml={10}>
                    {[<Link underline="hover" key="1" color="inherit" href="/devices">
                        Devices
                    </Link>,
                    <Link
                    underline="none"
                    key="2"
                    color="inherit"
                    hre={`/devices/${deviceID}`}
                    >
                    {deviceID}
                    </Link>]}
                    </Breadcrumbs>
                <Box sx={{
                display:'flex',
                justifyContent:'center',
                alignItems: 'center',
                gap: 2
                }}>
                    <Tabs value={router.query.id[1]}  aria-label="icon label tabs example" variant='scrollable'>
                        <Tab
                        value={"info"}
                        onClick={()=>{router.push(`/devices/usp/${deviceID}/info`)}}
                        icon={<SvgIcon><InformationCircleIcon/></SvgIcon>}
                        iconPosition={"end"}
                        label="Info" />
                        <Tab
                        value={"network"}
                        onClick={()=>{router.push(`/devices/usp/${deviceID}/network`)}}
                        icon={<SvgIcon><GlobeAltIcon/></SvgIcon>}
                        iconPosition={"end"}
                        label="Network" />
                        <Tab
                        value={"performance"}
                        onClick={()=>{router.push(`/devices/usp/${deviceID}/performance`)}}
                        icon={<SvgIcon><ChartBarIcon/></SvgIcon>}
                        iconPosition={"end"}
                        label="Performance" />
                        <Tab
                        value={"topology"}
                        onClick={()=>{router.push(`/devices/usp/${deviceID}/topology`)}}
                        icon={<SvgIcon><MapIcon/></SvgIcon>}
                        iconPosition={"end"}
                        label="Topology" />
                        <Tab
                        value={"discovery"}
                        onClick={()=>{router.push(`/devices/usp/${deviceID}/discovery`)}}
                        icon={<SvgIcon><MagnifyingGlassIcon/></SvgIcon>}
                        iconPosition={"end"}
                        label="Parameters" />
                        <Tab
                        value={"lcm"}
                        onClick={()=>{router.push(`/devices/usp/${deviceID}/lcm`)}}
                        icon={<SvgIcon><CubeTransparentIcon/></SvgIcon>}
                        iconPosition={"end"}
                        label="LCM" />
                        <Tab
                        value={"msg"}
                        onClick={()=>{router.push(`/devices/usp/${deviceID}/msg`)}}
                        icon={<SvgIcon><EnvelopeIcon/></SvgIcon>}
                        iconPosition={"end"}
                        label="Messages" />
                        <Tab
                        value={"history"}
                        onClick={()=>{router.push(`/devices/usp/${deviceID}/history`)}}
                        icon={<SvgIcon><ClockIcon/></SvgIcon>}
                        iconPosition={"end"}
                        label="History" />
                    </Tabs>
                    <Button
                        variant="outlined"
                        onClick={(e) => setUnimplementedAnchor(e.currentTarget)}
                        sx={{ minWidth: 150 }}
                    >
                        Unimplemented Features
                    </Button>
                    <Menu
                        anchorEl={unimplementedAnchor}
                        open={Boolean(unimplementedAnchor)}
                        onClose={() => setUnimplementedAnchor(null)}
                    >
                        <MenuItem disabled>
                            <SvgIcon sx={{ mr: 1 }}><WifiIcon/></SvgIcon>
                            Wi-Fi
                        </MenuItem>
                        <MenuItem disabled>
                            <SvgIcon sx={{ mr: 1 }}><SignalIcon/></SvgIcon>
                            Site Survey
                        </MenuItem>
                        <MenuItem disabled>
                            <SvgIcon sx={{ mr: 1 }}><DevicePhoneMobile/></SvgIcon>
                            Connected Devices
                        </MenuItem>
                        <MenuItem disabled>
                            <SvgIcon sx={{ mr: 1 }}><WrenchScrewDriverIcon/></SvgIcon>
                            Diagnostic
                        </MenuItem>
                        <MenuItem disabled>
                            <SvgIcon sx={{ mr: 1 }}><ServerStackIcon/></SvgIcon>
                            Ports
                        </MenuItem>
                        <MenuItem disabled>
                            <SvgIcon sx={{ mr: 1 }}><ArrowTrendingUp/></SvgIcon>
                            Historic
                        </MenuItem>
                        <MenuItem disabled>
                            <SvgIcon sx={{ mr: 1 }}><MapPin/></SvgIcon>
                            Location
                        </MenuItem>
                        <MenuItem disabled>
                            <SvgIcon sx={{ mr: 1 }}><CommandLineIcon/></SvgIcon>
                            Actions
                        </MenuItem>
                    </Menu>
                </Box>
                </Stack>
            </Container>
            <Container maxWidth="lg">
                <Stack spacing={3}>
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