import PropTypes from 'prop-types';
import {
  Box,
  Button,
  Card,
  Chip,
  CircularProgress,
  Dialog,
  DialogActions,
  DialogContent,
  DialogContentText,
  DialogTitle,
  IconButton,
  InputAdornment,
  SvgIcon,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableRow,
  TextField,
} from '@mui/material';
import { Scrollbar } from 'src/components/scrollbar';
import TrashIcon from '@heroicons/react/24/outline/TrashIcon';
import PauseIcon from '@heroicons/react/24/outline/PauseIcon';
import PlayIcon from '@heroicons/react/24/outline/PlayIcon';
import KeyIcon from '@heroicons/react/24/outline/KeyIcon';
import EyeIcon from '@heroicons/react/24/outline/EyeIcon';
import EyeSlashIcon from '@heroicons/react/24/outline/EyeSlashIcon';
import { useState } from 'react';
import { useAuth } from 'src/hooks/use-auth';

const statusColor = (status) => {
  switch (status) {
    case 'active':
      return 'success';
    case 'suspended':
      return 'warning';
    case 'disabled':
      return 'error';
    default:
      return 'default';
  }
};

export const TenantsTable = (props) => {
  const {
    items = [],
    onDelete,
    onToggleStatus,
  } = props;

  const auth = useAuth();
  const [showDeleteDialog, setShowDeleteDialog] = useState(false);
  const [tenantToDelete, setTenantToDelete] = useState(null);
  const [passwordDialogOpen, setPasswordDialogOpen] = useState(false);
  const [passwordTenant, setPasswordTenant] = useState(null);
  const [devicePassword, setDevicePassword] = useState('');
  const [passwordLoading, setPasswordLoading] = useState(false);
  const [showPassword, setShowPassword] = useState(false);
  const [passwordError, setPasswordError] = useState('');

  const getHeaders = () => {
    const h = new Headers();
    h.append('Content-Type', 'application/json');
    h.append('Authorization', auth.user.token);
    return h;
  };

  const openPasswordDialog = async (tenant) => {
    setPasswordTenant(tenant);
    setPasswordDialogOpen(true);
    setPasswordLoading(true);
    setPasswordError('');
    setDevicePassword('');
    try {
      const res = await fetch(
        `${process.env.NEXT_PUBLIC_REST_ENDPOINT || ''}/api/tenants/${tenant.slug}/device-password`,
        { method: 'GET', headers: getHeaders() }
      );
      if (res.ok) {
        const data = await res.json();
        setDevicePassword(data.password || '');
      }
    } catch (err) {
      console.error('Error fetching device password:', err);
    } finally {
      setPasswordLoading(false);
    }
  };

  const saveDevicePassword = async () => {
    if (!devicePassword) {
      setPasswordError('Password is required');
      return;
    }
    setPasswordLoading(true);
    setPasswordError('');
    try {
      const res = await fetch(
        `${process.env.NEXT_PUBLIC_REST_ENDPOINT || ''}/api/tenants/${passwordTenant.slug}/device-password`,
        {
          method: 'PUT',
          headers: getHeaders(),
          body: JSON.stringify({ password: devicePassword }),
        }
      );
      if (!res.ok) {
        const data = await res.json();
        setPasswordError(data.error || 'Failed to save password');
      } else {
        setPasswordDialogOpen(false);
      }
    } catch (err) {
      setPasswordError('Network error');
    } finally {
      setPasswordLoading(false);
    }
  };

  return (
    <Card>
      <Scrollbar>
        <Box sx={{ minWidth: 800 }}>
          <Table>
            <TableHead>
              <TableRow>
                <TableCell>Name</TableCell>
                <TableCell>Slug</TableCell>
                <TableCell>Status</TableCell>
                <TableCell>Created</TableCell>
                <TableCell>Actions</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {items.map((tenant) => (
                <TableRow hover key={tenant.slug}>
                  <TableCell>{tenant.name}</TableCell>
                  <TableCell>{tenant.slug}</TableCell>
                  <TableCell>
                    <Chip
                      label={tenant.status}
                      color={statusColor(tenant.status)}
                      size="small"
                    />
                  </TableCell>
                  <TableCell>
                    {tenant.created_at
                      ? new Date(tenant.created_at).toLocaleDateString()
                      : ''}
                  </TableCell>
                  <TableCell>
                    <Button
                      size="small"
                      onClick={() => openPasswordDialog(tenant)}
                      title="Device Password"
                    >
                      <SvgIcon color="action" fontSize="small">
                        <KeyIcon />
                      </SvgIcon>
                    </Button>
                    <Button
                      size="small"
                      onClick={() => {
                        onToggleStatus(
                          tenant.slug,
                          tenant.status === 'active' ? 'suspended' : 'active'
                        );
                      }}
                      title={
                        tenant.status === 'active' ? 'Suspend' : 'Activate'
                      }
                    >
                      <SvgIcon color="action" fontSize="small">
                        {tenant.status === 'active' ? (
                          <PauseIcon />
                        ) : (
                          <PlayIcon />
                        )}
                      </SvgIcon>
                    </Button>
                    <Button
                      size="small"
                      onClick={() => {
                        setTenantToDelete(tenant);
                        setShowDeleteDialog(true);
                      }}
                    >
                      <SvgIcon color="action" fontSize="small">
                        <TrashIcon />
                      </SvgIcon>
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </Box>
      </Scrollbar>
      <Dialog
        open={showDeleteDialog}
        onClose={() => setShowDeleteDialog(false)}
      >
        <DialogTitle>Delete Tenant</DialogTitle>
        <DialogContent>
          <DialogContentText>
            Are you sure you want to delete tenant &quot;{tenantToDelete?.name}
            &quot;? This action cannot be undone.
          </DialogContentText>
        </DialogContent>
        <DialogActions>
          <Button
            onClick={() => {
              setShowDeleteDialog(false);
              setTenantToDelete(null);
            }}
          >
            Cancel
          </Button>
          <Button
            color="error"
            onClick={() => {
              onDelete(tenantToDelete.slug);
              setShowDeleteDialog(false);
              setTenantToDelete(null);
            }}
          >
            Delete
          </Button>
        </DialogActions>
      </Dialog>
      <Dialog
        open={passwordDialogOpen}
        onClose={() => setPasswordDialogOpen(false)}
        maxWidth="sm"
        fullWidth
      >
        <DialogTitle>Device Password -- {passwordTenant?.name}</DialogTitle>
        <DialogContent>
          <DialogContentText sx={{ mb: 2 }}>
            Set a shared password for all devices connecting to this tenant.
            Per-device credentials (configured on the Credentials page) take precedence.
          </DialogContentText>
          {passwordLoading ? (
            <Box sx={{ display: 'flex', justifyContent: 'center', py: 2 }}>
              <CircularProgress size={24} />
            </Box>
          ) : (
            <TextField
              fullWidth
              label="Device Password"
              type={showPassword ? 'text' : 'password'}
              value={devicePassword}
              onChange={(e) => setDevicePassword(e.target.value)}
              error={!!passwordError}
              helperText={passwordError}
              slotProps={{
                input: {
                  endAdornment: (
                    <InputAdornment position="end">
                      <IconButton
                        onClick={() => setShowPassword(!showPassword)}
                        edge="end"
                        size="small"
                      >
                        <SvgIcon fontSize="small">
                          {showPassword ? <EyeSlashIcon /> : <EyeIcon />}
                        </SvgIcon>
                      </IconButton>
                    </InputAdornment>
                  ),
                },
              }}
            />
          )}
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setPasswordDialogOpen(false)}>Cancel</Button>
          <Button
            variant="contained"
            onClick={saveDevicePassword}
            disabled={passwordLoading}
          >
            Save
          </Button>
        </DialogActions>
      </Dialog>
    </Card>
  );
};

TenantsTable.propTypes = {
  items: PropTypes.array,
  onDelete: PropTypes.func,
  onToggleStatus: PropTypes.func,
};
