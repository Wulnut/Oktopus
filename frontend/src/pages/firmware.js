import { useCallback, useEffect, useState } from 'react';
import Head from 'next/head';
import {
  Box,
  Button,
  Container,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  Stack,
  SvgIcon,
  Typography,
} from '@mui/material';
import PlusIcon from '@heroicons/react/24/solid/PlusIcon';
import { Layout as DashboardLayout } from 'src/layouts/dashboard/layout';
import { FirmwareTable } from 'src/sections/firmware/firmware-table';
import { FirmwareUploadDialog } from 'src/sections/firmware/firmware-upload-dialog';
import { FirmwareEditDialog } from 'src/sections/firmware/firmware-edit-dialog';
import { useBackendContext } from 'src/contexts/backend-context';
import { useAlertContext } from 'src/contexts/error-context';

const Page = () => {
  const { httpRequest } = useBackendContext();
  const { setAlert } = useAlertContext();

  const [firmware, setFirmware] = useState([]);
  const [loading, setLoading] = useState(true);
  const [uploadDialogOpen, setUploadDialogOpen] = useState(false);

  // Edit dialog
  const [editDialogOpen, setEditDialogOpen] = useState(false);
  const [editingFirmware, setEditingFirmware] = useState(null);

  // Delete confirmation dialog
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [firmwareToDelete, setFirmwareToDelete] = useState(null);
  const [deleting, setDeleting] = useState(false);

  const fetchFirmware = useCallback(async () => {
    setLoading(true);
    try {
      const { status, result } = await httpRequest('/api/firmware', 'GET');
      if (status === 200 && Array.isArray(result)) {
        setFirmware(result);
      }
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchFirmware();
  }, [fetchFirmware]);

  const handleDeleteRequest = (id) => {
    setFirmwareToDelete(id);
    setDeleteDialogOpen(true);
  };

  const handleDeleteConfirm = async () => {
    if (!firmwareToDelete) return;
    setDeleting(true);
    try {
      const { status } = await httpRequest(`/api/firmware/${firmwareToDelete}`, 'DELETE');
      if (status === 204 || status === 200) {
        setFirmware((prev) => prev.filter((fw) => fw.id !== firmwareToDelete));
        setAlert({ severity: 'success', message: 'Firmware deleted successfully.' });
      }
    } finally {
      setDeleting(false);
      setDeleteDialogOpen(false);
      setFirmwareToDelete(null);
    }
  };

  const handlePhaseChange = async (id, newPhase) => {
    const body = JSON.stringify({ phase: newPhase });
    const headers = new Headers();
    headers.append('Content-Type', 'application/json');
    headers.append('Authorization', localStorage.getItem('token'));

    const { status } = await httpRequest(`/api/firmware/${id}/phase`, 'PUT', body, headers);
    if (status === 204 || status === 200) {
      setFirmware((prev) =>
        prev.map((fw) => (fw.id === id ? { ...fw, phase: newPhase } : fw))
      );
      setAlert({
        severity: 'success',
        message: `Firmware phase updated to "${newPhase === 'release' ? 'Release' : 'Internal Testing'}".`,
      });
    }
  };

  const handleEdit = (fw) => {
    setEditingFirmware(fw);
    setEditDialogOpen(true);
  };

  const handleEditSave = async (id, data) => {
    const { status } = await httpRequest(`/api/firmware/${id}`, 'PUT', JSON.stringify(data));
    if (status === 204 || status === 200) {
      setEditDialogOpen(false);
      setEditingFirmware(null);
      setAlert({ severity: 'success', message: 'Firmware updated.' });
      fetchFirmware();
    }
  };

  const handleUploadSuccess = () => {
    setUploadDialogOpen(false);
    setAlert({ severity: 'success', message: 'Firmware uploaded successfully.' });
    fetchFirmware();
  };

  return (
    <>
      <Head>
        <title>Firmware | Oktopus</title>
      </Head>
      <Box component="main" sx={{ flexGrow: 1, py: 8 }}>
        <Container maxWidth="xl">
          <Stack spacing={3}>
            <Stack direction="row" justifyContent="space-between" spacing={4}>
              <Typography variant="h4">Firmware Management</Typography>
              <Button
                startIcon={
                  <SvgIcon fontSize="small">
                    <PlusIcon />
                  </SvgIcon>
                }
                onClick={() => setUploadDialogOpen(true)}
                variant="contained"
              >
                Upload Firmware
              </Button>
            </Stack>

            <FirmwareTable
              firmware={firmware}
              loading={loading}
              onEdit={handleEdit}
              onDelete={handleDeleteRequest}
              onPhaseChange={handlePhaseChange}
            />
          </Stack>
        </Container>
      </Box>

      <FirmwareEditDialog
        open={editDialogOpen}
        onClose={() => { setEditDialogOpen(false); setEditingFirmware(null); }}
        onSave={handleEditSave}
        firmware={editingFirmware}
      />

      <FirmwareUploadDialog
        open={uploadDialogOpen}
        onClose={() => setUploadDialogOpen(false)}
        onSuccess={handleUploadSuccess}
      />

      {/* Delete confirmation dialog */}
      <Dialog
        open={deleteDialogOpen}
        onClose={() => {
          if (!deleting) {
            setDeleteDialogOpen(false);
            setFirmwareToDelete(null);
          }
        }}
        maxWidth="sm"
        fullWidth
      >
        <DialogTitle>Delete Firmware</DialogTitle>
        <DialogContent>
          <Typography>
            Are you sure you want to delete this firmware? This action cannot be undone.
          </Typography>
        </DialogContent>
        <DialogActions>
          <Button
            onClick={() => {
              setDeleteDialogOpen(false);
              setFirmwareToDelete(null);
            }}
            disabled={deleting}
          >
            Cancel
          </Button>
          <Button
            onClick={handleDeleteConfirm}
            variant="contained"
            color="error"
            disabled={deleting}
          >
            {deleting ? 'Deleting...' : 'Delete'}
          </Button>
        </DialogActions>
      </Dialog>
    </>
  );
};

Page.getLayout = (page) => <DashboardLayout>{page}</DashboardLayout>;

export default Page;
