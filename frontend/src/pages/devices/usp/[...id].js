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
            default:
                router.replace(`/devices/usp/${deviceID}/discovery`)
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