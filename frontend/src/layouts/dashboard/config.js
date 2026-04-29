import ChartBarIcon from '@heroicons/react/24/solid/ChartBarIcon';
import CogIcon from '@heroicons/react/24/solid/CogIcon';
import RectangleGroupIcon from '@heroicons/react/24/solid/RectangleGroupIcon'
import ArrowDownOnSquareStackIcon from '@heroicons/react/24/solid/ArrowDownOnSquareStackIcon'
import KeyIcon from '@heroicons/react/24/solid/KeyIcon'
import CpuChip from '@heroicons/react/24/solid/CpuChipIcon';
import { SvgIcon } from '@mui/material';
import UserIcon from '@heroicons/react/24/solid/UserIcon';
import BookOpenIcon from '@heroicons/react/24/solid/BookOpenIcon';
import CommandLineIcon from '@heroicons/react/24/solid/CommandLineIcon';
import CubeIcon from '@heroicons/react/24/solid/CubeIcon';
import CircleStackIcon from '@heroicons/react/24/solid/CircleStackIcon';
import InformationCircleIcon from '@heroicons/react/24/outline/InformationCircleIcon';
import GlobeAltIcon from '@heroicons/react/24/outline/GlobeAltIcon';
import LinkIcon from '@heroicons/react/24/outline/LinkIcon';
import ChartBarIconOutline from '@heroicons/react/24/outline/ChartBarIcon';
import MapIcon from '@heroicons/react/24/outline/MapIcon';
import MagnifyingGlassIcon from '@heroicons/react/24/solid/MagnifyingGlassIcon';
import CubeTransparentIcon from '@heroicons/react/24/outline/CubeTransparentIcon';
import EnvelopeIconOutline from '@heroicons/react/24/outline/EnvelopeIcon';
import ClockIcon from '@heroicons/react/24/outline/ClockIcon';
import BuildingOfficeIcon from '@heroicons/react/24/solid/BuildingOfficeIcon';

export const getDeviceSubItems = (protocol, deviceID) => [
  {
    title: 'Info',
    path: `/devices/${protocol}/${deviceID}/info`,
    icon: <SvgIcon fontSize="small"><InformationCircleIcon /></SvgIcon>,
  },
  {
    title: 'Data Model',
    path: `/devices/${protocol}/${deviceID}/discovery`,
    icon: <SvgIcon fontSize="small"><MagnifyingGlassIcon /></SvgIcon>,
  },
  {
    title: 'Network',
    path: `/devices/${protocol}/${deviceID}/network`,
    icon: <SvgIcon fontSize="small"><GlobeAltIcon /></SvgIcon>,
  },
  {
    title: 'Bridging',
    path: `/devices/${protocol}/${deviceID}/bridging`,
    icon: <SvgIcon fontSize="small"><LinkIcon /></SvgIcon>,
  },
  {
    title: 'Performance',
    path: `/devices/${protocol}/${deviceID}/performance`,
    icon: <SvgIcon fontSize="small"><ChartBarIconOutline /></SvgIcon>,
  },
  {
    title: 'Topology',
    path: `/devices/${protocol}/${deviceID}/topology`,
    icon: <SvgIcon fontSize="small"><MapIcon /></SvgIcon>,
  },
  {
    title: 'LCM',
    path: `/devices/${protocol}/${deviceID}/lcm`,
    icon: <SvgIcon fontSize="small"><CubeTransparentIcon /></SvgIcon>,
  },
  {
    title: 'Messages',
    path: `/devices/${protocol}/${deviceID}/msg`,
    icon: <SvgIcon fontSize="small"><EnvelopeIconOutline /></SvgIcon>,
  },
  {
    title: 'History',
    path: `/devices/${protocol}/${deviceID}/history`,
    icon: <SvgIcon fontSize="small"><ClockIcon /></SvgIcon>,
  },
];

export const items = [
  {
    title: 'Tenants',
    path: '/tenants',
    icon: (
      <SvgIcon fontSize="small">
        <BuildingOfficeIcon />
      </SvgIcon>
    ),
    superAdminOnly: true,
  },
  {
    title: 'Overview',
    path: '/',
    icon: (
      <SvgIcon fontSize="small">
        <ChartBarIcon />
      </SvgIcon>
    )
  },
  {
    title: 'Devices',
    path: '/devices',
    icon: (
      <SvgIcon fontSize="small">
        <CpuChip />
      </SvgIcon>
    )
  },
  {
    title: 'Firmware',
    path: '/firmware',
    icon: (
      <SvgIcon fontSize="small">
        <CircleStackIcon />
      </SvgIcon>
    )
  },
  {
    title: 'Device Credentials',
    path: '/credentials',
    icon: (
      <SvgIcon fontSize="small">
        <KeyIcon />
      </SvgIcon>
    )
  },
  {
    title: 'Containers Store',
    path: '/containers-store',
    icon: (
      <SvgIcon fontSize="small">
        <CubeIcon />
      </SvgIcon>
    )
  },
  {
    title: 'Settings',
    path: '/settings',
    icon: (
      <SvgIcon fontSize="small">
        <CogIcon />
      </SvgIcon>
    )
  },
  {
    title: 'Mass Actions',
    icon: (
      <SvgIcon fontSize="small">
        <RectangleGroupIcon />
      </SvgIcon>
    ),
    children: [
      {
        title: 'Firmware Update',
        path: '/mass-actions/firmware',
        icon: (
          <SvgIcon fontSize="small">
            <ArrowDownOnSquareStackIcon />
          </SvgIcon>
        ),
      },
      {
        title: 'Scripts',
        path: '/mass-actions/scripts',
        icon: (
          <SvgIcon fontSize="small">
            <CommandLineIcon />
          </SvgIcon>
        ),
      },
    ]
  },
  {
    title: 'Scripts',
    path: '/scripts',
    icon: (
      <SvgIcon fontSize="small">
        <CommandLineIcon />
      </SvgIcon>
    )
  },
  {
    title: 'Users',
    path: '/users',
    icon: (
      <SvgIcon fontSize="small">
        <UserIcon />
      </SvgIcon>
    )
  },
];

/*
  {
    title: 'Customers',
    path: '/customers',
    icon: (
      <SvgIcon fontSize="small">
        <UsersIcon />
      </SvgIcon>
    )
  },
    {
    title: 'Account',
    path: '/account',
    icon: (
      <SvgIcon fontSize="small">
        <UserIcon />
      </SvgIcon>
    )
  },
  {
    title: 'Register',
    path: '/auth/register',
    icon: (
      <SvgIcon fontSize="small">
        <UserPlusIcon />
      </SvgIcon>
    )
  },
  {
    title: 'Login',
    path: '/auth/login',
    icon: (
      <SvgIcon fontSize="small">
        <LockClosedIcon />
      </SvgIcon>
    )
  },
  {
    title: 'Companies',
    path: '/companies',
    icon: (
      <SvgIcon fontSize="small">
        <ShoppingBagIcon />
      </SvgIcon>
    )
  },
  {
    title: 'Error',
    path: '/404',
    icon: (
      <SvgIcon fontSize="small">
        <XCircleIcon />
      </SvgIcon>
    )
  }
*/ 
