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
import { ScriptsTable } from 'src/sections/scripts/scripts-table';
import { ScriptEditorDialog } from 'src/sections/scripts/script-editor-dialog';
import { ScriptExecuteDialog } from 'src/sections/scripts/script-execute-dialog';
import { ScriptHistoryDialog } from 'src/sections/scripts/script-history-dialog';
import { useBackendContext } from 'src/contexts/backend-context';
import { useAlertContext } from 'src/contexts/error-context';

const Page = () => {
  const { httpRequest } = useBackendContext();
  const { setAlert } = useAlertContext();

  const [scripts, setScripts] = useState([]);
  const [loading, setLoading] = useState(true);

  // Editor dialog
  const [editorOpen, setEditorOpen] = useState(false);
  const [editingScript, setEditingScript] = useState(null);

  // Execute dialog
  const [executeOpen, setExecuteOpen] = useState(false);
  const [executingScript, setExecutingScript] = useState(null);

  // History dialog
  const [historyOpen, setHistoryOpen] = useState(false);
  const [historyScript, setHistoryScript] = useState(null);

  // Delete confirmation
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [scriptToDelete, setScriptToDelete] = useState(null);
  const [deleting, setDeleting] = useState(false);

  const fetchScripts = useCallback(async () => {
    setLoading(true);
    try {
      const { status, result } = await httpRequest('/api/scripts', 'GET');
      if (status === 200 && Array.isArray(result)) {
        setScripts(result);
      }
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchScripts();
  }, [fetchScripts]);

  const handleCreate = () => {
    setEditingScript(null);
    setEditorOpen(true);
  };

  const handleEdit = (script) => {
    setEditingScript(script);
    setEditorOpen(true);
  };

  const handleDuplicate = (script) => {
    setEditingScript({
      ...script,
      id: undefined,
      name: script.name + ' (copy)',
      builtin: false,
    });
    setEditorOpen(true);
  };

  const handleEditorSave = async (scriptData) => {
    const isEdit = editingScript && editingScript.id;
    const method = isEdit ? 'PUT' : 'POST';
    const path = isEdit ? `/api/scripts/${editingScript.id}` : '/api/scripts';

    const { status } = await httpRequest(path, method, JSON.stringify(scriptData));
    if (status === 201 || status === 204 || status === 200) {
      setEditorOpen(false);
      setEditingScript(null);
      setAlert({ severity: 'success', message: isEdit ? 'Script updated.' : 'Script created.' });
      fetchScripts();
    }
  };

  const handleExecute = (script) => {
    setExecutingScript(script);
    setExecuteOpen(true);
  };

  const handleHistory = (script) => {
    setHistoryScript(script);
    setHistoryOpen(true);
  };

  const handleDeleteRequest = (script) => {
    setScriptToDelete(script);
    setDeleteDialogOpen(true);
  };

  const handleDeleteConfirm = async () => {
    if (!scriptToDelete) return;
    setDeleting(true);
    try {
      const { status } = await httpRequest(`/api/scripts/${scriptToDelete.id}`, 'DELETE');
      if (status === 204 || status === 200) {
        setScripts((prev) => prev.filter((s) => s.id !== scriptToDelete.id));
        setAlert({ severity: 'success', message: 'Script deleted.' });
      }
    } finally {
      setDeleting(false);
      setDeleteDialogOpen(false);
      setScriptToDelete(null);
    }
  };

  return (
    <>
      <Head>
        <title>Scripts | Oktopus</title>
      </Head>
      <Box component="main" sx={{ flexGrow: 1, py: 8 }}>
        <Container maxWidth="xl">
          <Stack spacing={3}>
            <Stack direction="row" justifyContent="space-between" spacing={4}>
              <Typography variant="h4">Scripts</Typography>
              <Button
                startIcon={
                  <SvgIcon fontSize="small">
                    <PlusIcon />
                  </SvgIcon>
                }
                onClick={handleCreate}
                variant="contained"
              >
                New Script
              </Button>
            </Stack>

            <ScriptsTable
              scripts={scripts}
              loading={loading}
              onEdit={handleEdit}
              onDuplicate={handleDuplicate}
              onExecute={handleExecute}
              onHistory={handleHistory}
              onDelete={handleDeleteRequest}
            />
          </Stack>
        </Container>
      </Box>

      <ScriptEditorDialog
        open={editorOpen}
        onClose={() => { setEditorOpen(false); setEditingScript(null); }}
        onSave={handleEditorSave}
        script={editingScript}
      />

      <ScriptExecuteDialog
        open={executeOpen}
        onClose={() => { setExecuteOpen(false); setExecutingScript(null); }}
        script={executingScript}
      />

      <ScriptHistoryDialog
        open={historyOpen}
        onClose={() => { setHistoryOpen(false); setHistoryScript(null); }}
        script={historyScript}
      />

      <Dialog
        open={deleteDialogOpen}
        onClose={() => {
          if (!deleting) {
            setDeleteDialogOpen(false);
            setScriptToDelete(null);
          }
        }}
        maxWidth="sm"
        fullWidth
      >
        <DialogTitle>Delete Script</DialogTitle>
        <DialogContent>
          <Typography>
            Are you sure you want to delete "{scriptToDelete?.name}"? This action cannot be undone.
          </Typography>
        </DialogContent>
        <DialogActions>
          <Button
            onClick={() => { setDeleteDialogOpen(false); setScriptToDelete(null); }}
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
