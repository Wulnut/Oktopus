import NextLink from 'next/link';
import { usePathname } from 'next/navigation';
import PropTypes from 'prop-types';
import Link from 'next/link'
import {
  Box,
  Button,
  Collapse,
  Divider,
  Drawer,
  List,
  ListItemButton,
  ListItemText,
  Stack,
  SvgIcon,
  Typography,
  useMediaQuery
} from '@mui/material';
import { Logo } from 'src/components/logo';
import { Scrollbar } from 'src/components/scrollbar';
import { items, getDeviceSubItems } from './config';
import { SideNavItem } from './side-nav-item';
import { useTheme } from '@mui/material';
import { useTenant } from 'src/contexts/tenant-context';
import { useAppVersion } from 'src/hooks/use-app-version';

export const SideNav = (props) => {
  const { open, onClose } = props;
  const pathname = usePathname();
  const lgUp = useMediaQuery((theme) => theme.breakpoints.up('lg'));
  const { isSuperAdmin } = useTenant();
  const appVersion = useAppVersion();

  const theme = useTheme();

  const filteredItems = items.filter((item) => {
    if (item.superAdminOnly && !isSuperAdmin) return false;
    return true;
  });

  const isItemActive = (currentPath, itemPath) => {
    if (!itemPath) return false;
    if (currentPath === itemPath) {
      return true;
    }

    if (currentPath.includes(itemPath) && itemPath !== '/') {
      return true;
    }

    return false;
  }

  // Extract device info from pathname for sub-tabs
  const deviceMatch = pathname?.match(/^\/devices\/(usp|cwmp)\/([^/]+)/);
  const deviceProtocol = deviceMatch?.[1];
  const deviceID = deviceMatch?.[2];
  const deviceSubItems = deviceProtocol && deviceID ? getDeviceSubItems(deviceProtocol, deviceID) : [];

  const content = (
    <Scrollbar
      sx={{
        height: '100%',
        '& .simplebar-content': {
          height: '100%'
        },
        '& .simplebar-scrollbar:before': {
          background: 'neutral.400'
        }
      }}
    >
      <Box
        sx={{
          display: 'flex',
          flexDirection: 'column',
          height: '100%'
        }}
      >
        <Box sx={{ p: 1 }}>
          <Box
            sx={{
              alignItems: 'center',
              backgroundColor: 'rgba(255, 255, 255, 0.04)',
              borderRadius: 1,
              cursor: 'pointer',
              display: 'flex',
              justifyContent: 'space-between',
              mt: 2,
              p: '12px'
            }}
          >
            <Link href={typeof window !== 'undefined' ? `${window.location.origin}/devices` : '/devices'}>
              <div style={{display:'flex',justifyContent:'center'}}>
                <img src={`${process.env.NEXT_PUBLIC_REST_ENDPOINT || ""}/images/logo.png`}
                style={{ width: '30%' }}
                />
              </div>
            </Link>
          </Box>
        </Box>
        <Divider sx={{ borderColor: 'neutral.700' }} />
        <Box
          component="nav"
          sx={{
            flexGrow: 1,
            px: 2,
            py: 3
          }}
        >
          <Stack
            component="ul"
            spacing={0.5}
            sx={{
              listStyle: 'none',
              p: 0,
              m: 0
            }}
          >
            {filteredItems.map((item) => {
              const active = isItemActive(pathname, item.path);
              // Inject device sub-items as children of the Devices item
              const itemChildren = item.path === '/devices' && deviceSubItems.length > 0
                ? deviceSubItems.map((sub) => ({
                    title: sub.title,
                    path: sub.path,
                    icon: sub.icon,
                  }))
                : item?.children;

              return (
                <SideNavItem
                  active={active}
                  disabled={item.disabled}
                  external={item.external}
                  icon={item.icon}
                  key={item.title}
                  path={item.path}
                  title={item.title}
                  padleft={2}
                  tooltip={item.tooltip}
                >
                  {itemChildren}
                </SideNavItem>
              );
            })}
            <Collapse in={open} timeout="auto" unmountOnExit>
              <Box
                component="span"
                sx={{
                  color: 'neutral.400',
                  flexGrow: 1,
                  fontFamily: (theme) => theme.typography.fontFamily,
                  fontSize: 14,
                  fontWeight: 600,
                  lineHeight: '24px',
                  whiteSpace: 'nowrap',
                  ...(true && {
                    color: 'common.white'
                  }),
                  ...(false && {
                    color: 'neutral.500'
                  })
                }}
              >
                {""}
              </Box>
            </Collapse>
            {/* <List>
              <Collapse in={true}>
                <List disablePadding>
                <ListItemButton sx={{ pl: 4 }}>
                  <ListItemText primary="Starred" />
                  oi
                </ListItemButton>
                </List>
              </Collapse>
            </List> */}
          </Stack>
        </Box>
        <Box
          sx={{
            mt: 'auto',
            px: 2,
            py: 2
          }}
        >
          <Typography
            color="neutral.400"
            variant="caption"
            component="div"
            sx={{ lineHeight: 1.4 }}
            title={appVersion.built_at ? `Built ${appVersion.built_at}` : undefined}
          >
            {appVersion.label}
          </Typography>
        </Box>
      </Box>
    </Scrollbar>
  );

  if (lgUp) {
    return (
      <Drawer
        anchor="left"
        open
        PaperProps={{
          sx: {
            background: theme.palette.mode === 'dark'
              ? `linear-gradient(120deg, ${theme.palette.background.paper} 0%, #1a2332 90%)`
              : `linear-gradient(120deg, ${theme.palette.neutral["800"]} 0%, ${theme.palette.primary.dark} 90%)`,
            width: 280
          }
        }}
        variant="permanent"
      >
        {content}
      </Drawer>
    );
  }

  return (
    <Drawer
      anchor="left"
      onClose={onClose}
      open={open}
      PaperProps={{
        sx: {
          background: theme.palette.mode === 'dark'
            ? `linear-gradient(120deg, ${theme.palette.background.paper} 0%, #1a2332 90%)`
            : `linear-gradient(120deg, ${theme.palette.neutral["800"]} 0%, ${theme.palette.primary.dark} 90%)`,
          color: 'common.white',
          width: 280
        }
      }}
      sx={{ zIndex: (theme) => theme.zIndex.appBar + 100 }}
      variant="temporary"
    >
      {content}
    </Drawer>
  );
};

SideNav.propTypes = {
  onClose: PropTypes.func,
  open: PropTypes.bool,
};
