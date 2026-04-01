import PropTypes from 'prop-types';
import {
  Box,
  Button,
  Card,
  Chip,
  Dialog,
  DialogActions,
  DialogContent,
  DialogContentText,
  DialogTitle,
  SvgIcon,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableRow,
} from '@mui/material';
import { Scrollbar } from 'src/components/scrollbar';
import TrashIcon from '@heroicons/react/24/outline/TrashIcon';
import PauseIcon from '@heroicons/react/24/outline/PauseIcon';
import PlayIcon from '@heroicons/react/24/outline/PlayIcon';
import { useState } from 'react';

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

  const [showDeleteDialog, setShowDeleteDialog] = useState(false);
  const [tenantToDelete, setTenantToDelete] = useState(null);

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
    </Card>
  );
};

TenantsTable.propTypes = {
  items: PropTypes.array,
  onDelete: PropTypes.func,
  onToggleStatus: PropTypes.func,
};
