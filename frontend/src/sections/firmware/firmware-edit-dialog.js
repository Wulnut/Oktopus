import { useEffect, useState } from 'react';
import {
  Button,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  Stack,
  TextField,
} from '@mui/material';

export const FirmwareEditDialog = ({ open, onClose, onSave, firmware }) => {
  const [name, setName] = useState('');
  const [vendor, setVendor] = useState('');
  const [model, setModel] = useState('');
  const [buildVersion, setBuildVersion] = useState('');
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (open && firmware) {
      setName(firmware.name || '');
      setVendor(firmware.vendor || '');
      setModel(firmware.model || '');
      setBuildVersion(firmware.build_version || '');
    }
  }, [open, firmware]);

  const handleSave = async () => {
    setSaving(true);
    try {
      await onSave(firmware.id, {
        name: name.trim(),
        vendor: vendor.trim(),
        model: model.trim(),
        build_version: buildVersion.trim(),
      });
    } finally {
      setSaving(false);
    }
  };

  return (
    <Dialog open={open} onClose={saving ? undefined : onClose} maxWidth="sm" fullWidth>
      <DialogTitle>Edit Firmware</DialogTitle>
      <DialogContent sx={{ pt: 3 }}>
        <Stack spacing={3} sx={{ mt: 1 }}>
          <TextField
            label="Name"
            value={name}
            onChange={(e) => setName(e.target.value)}
            fullWidth
            required
            id="fw-edit-name"
            autoComplete="off"
          />
          <Stack direction="row" spacing={2}>
            <TextField
              label="Vendor"
              value={vendor}
              onChange={(e) => setVendor(e.target.value)}
              fullWidth
              id="fw-edit-vendor"
              autoComplete="off"
            />
            <TextField
              label="Model"
              value={model}
              onChange={(e) => setModel(e.target.value)}
              fullWidth
              id="fw-edit-model"
              autoComplete="off"
            />
          </Stack>
          <TextField
            label="Build Version"
            value={buildVersion}
            onChange={(e) => setBuildVersion(e.target.value)}
            fullWidth
            required
            id="fw-edit-version"
            autoComplete="off"
          />
        </Stack>
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose} disabled={saving}>Cancel</Button>
        <Button
          onClick={handleSave}
          variant="contained"
          disabled={saving || !name.trim() || !buildVersion.trim()}
        >
          {saving ? 'Saving...' : 'Save'}
        </Button>
      </DialogActions>
    </Dialog>
  );
};
