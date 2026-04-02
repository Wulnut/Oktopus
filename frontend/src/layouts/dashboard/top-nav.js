import PropTypes from 'prop-types';
import BellIcon from '@heroicons/react/24/solid/BellIcon';
import UsersIcon from '@heroicons/react/24/solid/UsersIcon';
import PhoneIcon from '@heroicons/react/24/solid/PhoneIcon';
import Bars3Icon from '@heroicons/react/24/solid/Bars3Icon';
import MagnifyingGlassIcon from '@heroicons/react/24/solid/MagnifyingGlassIcon';
import {
  Avatar,
  Badge,
  Box,
  Chip,
  FormControl,
  IconButton,
  InputLabel,
  MenuItem,
  Select,
  Stack,
  SvgIcon,
  Tooltip,
  Typography,
  useMediaQuery,
  Dialog,
  DialogTitle,
  DialogActions,
  DialogContent,
  DialogContentText,
  Button,
  Link
} from '@mui/material';
import XMarkIcon from '@heroicons/react/24/outline/XMarkIcon';
import { alpha } from '@mui/material/styles';
import { usePopover } from 'src/hooks/use-popover';
import { AccountPopover } from './account-popover';
import { useAuth } from 'src/hooks/use-auth';
import { useTenant } from 'src/contexts/tenant-context';
import { WsContext } from 'src/contexts/socketio-context';
import { useContext, useEffect, useState } from 'react';

const SIDE_NAV_WIDTH = 280;
const TOP_NAV_HEIGHT = 64;

export const TopNav = (props) => {
  const { onNavOpen } = props;
  const lgUp = useMediaQuery((theme) => theme.breakpoints.up('lg'));
  const accountPopover = usePopover();
  const auth = useAuth();
  const { isSuperAdmin, tenantSlug, setActiveTenant } = useTenant();
  const { answerCall, call, callAccepted } = useContext(WsContext);
  const [tenantList, setTenantList] = useState([]);

  useEffect(() => {
    if (!isSuperAdmin || !auth.user?.token) return;
    const h = new Headers();
    h.append('Content-Type', 'application/json');
    h.append('Authorization', auth.user.token);
    fetch(`${process.env.NEXT_PUBLIC_REST_ENDPOINT || ''}/api/tenants`, {
      method: 'GET',
      headers: h,
      redirect: 'follow',
    })
      .then((res) => (res.ok ? res.json() : []))
      .then((data) => setTenantList(data || []))
      .catch(() => {});
  }, [isSuperAdmin, auth.user?.token]);

  return ( auth.user &&
    <>
      <Box
        component="header"
        sx={{
          backdropFilter: 'blur(6px)',
          backgroundColor: (theme) => alpha(theme.palette.background.default, 0.8),
          position: 'sticky',
          left: {
            lg: `${SIDE_NAV_WIDTH}px`
          },
          top: 0,
          width: {
            lg: `calc(100% - ${SIDE_NAV_WIDTH}px)`
          },
          zIndex: (theme) => theme.zIndex.appBar
        }}
      >
        <Stack
          alignItems="center"
          direction="row"
          justifyContent="space-between"
          spacing={2}
          sx={{
            minHeight: TOP_NAV_HEIGHT,
            px: 2
          }}
        >
          <Stack
            alignItems="center"
            direction="row"
            spacing={2}
          >
            {!lgUp && (
              <IconButton onClick={onNavOpen}>
                <SvgIcon fontSize="small">
                  <Bars3Icon />
                </SvgIcon>
              </IconButton>
            )}
            {isSuperAdmin && (
              <FormControl size="small" sx={{ minWidth: 220 }}>
                <InputLabel id="tenant-selector-label" shrink>Active Tenant</InputLabel>
                <Select
                  labelId="tenant-selector-label"
                  value={tenantSlug}
                  label="Active Tenant"
                  onChange={(e) => setActiveTenant(e.target.value)}
                  displayEmpty
                  notched
                  renderValue={(selected) => {
                    if (!selected) {
                      return <Typography variant="body2" color="text.secondary">None (Provider view)</Typography>;
                    }
                    const t = tenantList.find((item) => item.slug === selected);
                    return t ? t.name : selected;
                  }}
                >
                  <MenuItem value="">
                    <em>None (Provider view)</em>
                  </MenuItem>
                  {tenantList.map((t) => (
                    <MenuItem key={t.slug} value={t.slug}>
                      {t.name} ({t.slug})
                    </MenuItem>
                  ))}
                </Select>
              </FormControl>
            )}
          </Stack>
          <Stack
            alignItems="center"
            direction="row"
            spacing={2}
          >
            {/* <Tooltip title="Contacts">
              <IconButton>
                <SvgIcon fontSize="small">
                  <UsersIcon />
                </SvgIcon>
              </IconButton>
            </Tooltip> */}
            {/*<Tooltip title="Notifications">
              <IconButton>
                <Badge
                  badgeContent={4}
                  color="success"
                  variant="dot"
                >
                  <SvgIcon fontSize="small">
                    <BellIcon />
                  </SvgIcon>
                </Badge>
              </IconButton>
            </Tooltip>*/}
            <Avatar
              onClick={accountPopover.handleOpen}
              ref={accountPopover.anchorRef}
              sx={{
                cursor: 'pointer',
                height: 40,
                width: 40
              }}
              src={auth.user.avatar}
            />
          </Stack>
        </Stack>
      </Box>
      {/* {call.isReceivingCall && !callAccepted &&
      <Dialog
        fullWidth={ true } 
        maxWidth={"sm"}
        open={true}
        //scroll={scroll}
        aria-labelledby="scroll-dialog-title"
        aria-describedby="scroll-dialog-description"
      >
        <DialogContent dividers={scroll === 'paper'}>
        <Box
        display="flex" 
        alignItems="center" 
        justifyContent={'center'}
        >
              <Box sx={{margin:"30px",textAlign:'center'}}>
                <Avatar
                  sx={{
                      height: 150,
                      width: 150,
                  }}
                  src={"/assets/avatars/default-avatar.png"}
                  />
                  <Box flexGrow={1} >{call.from}</Box>
              </Box>
        </Box>
        <Box display="flex" 
        alignItems="center" 
        justifyContent={'center'}>
        <IconButton>
          <Tooltip title="Refuse" 
          placement="left" 
          onClick={()=>{}}>
              <SvgIcon
              sx={{cursor:'pointer'}}
              style={{transform: "scale(1.5,1.5)"}}
              >
                <PhoneIcon 
                color={"#CB1E02"}
                />
              </SvgIcon>
          </Tooltip>
        </IconButton>
          <div style={{width:'15%'}}></div>
          <IconButton>
            <Tooltip title="Accept" 
            placement="right" 
            onClick={()=>{}}>
                <SvgIcon
                sx={{cursor:'pointer'}}
                style={{transform: "scale(1.5,1.5) scale(-1,1)"}}
                >
                  <PhoneIcon 
                  color={"#17A000"}
                  />
                </SvgIcon>
            </Tooltip>
          </IconButton>
        </Box>
        </DialogContent>
      </Dialog>} */}
      <AccountPopover
        anchorEl={accountPopover.anchorRef.current}
        open={accountPopover.open}
        onClose={accountPopover.handleClose}
      />
    </>
  );
};

TopNav.propTypes = {
  onNavOpen: PropTypes.func
};
