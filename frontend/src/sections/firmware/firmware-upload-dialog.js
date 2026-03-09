import { useRef, useState } from 'react';
import PropTypes from 'prop-types';
import {
  Alert,
  Box,
  Button,
  CircularProgress,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  FormControl,
  IconButton,
  InputLabel,
  MenuItem,
  Select,
  Stack,
  SvgIcon,
  Tab,
  Tabs,
  TextField,
  Typography,
} from '@mui/material';
import XMarkIcon from '@heroicons/react/24/outline/XMarkIcon';
import CloudArrowUpIcon from '@heroicons/react/24/outline/CloudArrowUpIcon';
import { useBackendContext } from 'src/contexts/backend-context';

const PHASES = [
  { value: 'internal_testing', label: 'Internal Testing' },
  { value: 'release', label: 'Release' },
];

export const FirmwareUploadDialog = ({ open, onClose, onSuccess }) => {
  const { httpRequest } = useBackendContext();
  const fileInputRef = useRef(null);

  const [tab, setTab] = useState(0); // 0 = Upload File, 1 = External URL
  const [name, setName] = useState('');
  const [buildVersion, setBuildVersion] = useState('');
  const [phase, setPhase] = useState('internal_testing');
  const [file, setFile] = useState(null);
  const [downloadUrl, setDownloadUrl] = useState('');
  const [fileName, setFileName] = useState('');
  const [fingerprint, setFingerprint] = useState('');
  const [uploading, setUploading] = useState(false);
  const [error, setError] = useState(null);

  const resetForm = () => {
    setTab(0);
    setName('');
    setBuildVersion('');
    setPhase('internal_testing');
    setFile(null);
    setDownloadUrl('');
    setFileName('');
    setFingerprint('');
    setError(null);
  };

  const handleClose = () => {
    if (uploading) return;
    resetForm();
    onClose();
  };

  const handleFileChange = (e) => {
    const selected = e.target.files[0];
    if (selected) {
      setFile(selected);
      setError(null);
    }
    // Reset input so same file can be re-selected
    e.target.value = '';
  };

  const handleSubmit = async () => {
    if (!name.trim()) {
      setError('Name is required.');
      return;
    }
    if (!buildVersion.trim()) {
      setError('Build version is required.');
      return;
    }
    if (tab === 0 && !file) {
      setError('Please select a firmware file.');
      return;
    }
    if (tab === 1 && !downloadUrl.trim()) {
      setError('Download URL is required.');
      return;
    }

    setUploading(true);
    setError(null);

    try {
      const formData = new FormData();
      formData.append('name', name.trim());
      formData.append('build_version', buildVersion.trim());
      formData.append('phase', phase);

      if (tab === 0) {
        formData.append('file', file);
      } else {
        formData.append('download_url', downloadUrl.trim());
        if (fileName.trim()) {
          formData.append('file_name', fileName.trim());
        }
        if (fingerprint.trim()) {
          formData.append('fingerprint', fingerprint.trim());
        }
      }

      // Build headers with Authorization only — no Content-Type so browser sets multipart boundary
      const headers = new Headers();
      headers.append('Authorization', localStorage.getItem('token'));

      const { status } = await httpRequest('/api/firmware', 'POST', formData, headers);

      if (status === 200 || status === 201) {
        resetForm();
        onSuccess && onSuccess();
      }
      // Non-2xx errors are handled by httpRequest (sets alert)
    } catch (err) {
      setError(err.message || 'An error occurred while uploading firmware.');
    } finally {
      setUploading(false);
    }
  };

  return (
    <Dialog open={open} onClose={handleClose} maxWidth="sm" fullWidth>
      <DialogTitle>
        <Box display="flex" justifyContent="space-between" alignItems="center">
          <Typography variant="h6">Upload Firmware</Typography>
          <IconButton onClick={handleClose} disabled={uploading} size="small">
            <SvgIcon>
              <XMarkIcon />
            </SvgIcon>
          </IconButton>
        </Box>
      </DialogTitle>

      <DialogContent>
        <Stack spacing={3} mt={1}>
          {error && (
            <Alert severity="error" onClose={() => setError(null)}>
              {error}
            </Alert>
          )}

          {/* Basic fields */}
          <TextField
            id="fw-name"
            label="Name"
            variant="outlined"
            fullWidth
            required
            value={name}
            onChange={(e) => setName(e.target.value)}
            disabled={uploading}
            placeholder="e.g., Router Firmware v2.1"
            autoComplete="off"
          />

          <TextField
            id="fw-build-version"
            label="Build Version"
            variant="outlined"
            fullWidth
            required
            value={buildVersion}
            onChange={(e) => setBuildVersion(e.target.value)}
            disabled={uploading}
            placeholder="e.g., 2.1.0-stable"
            autoComplete="off"
          />

          <FormControl fullWidth>
            <InputLabel id="phase-select-label" htmlFor="fw-phase-select">Phase</InputLabel>
            <Select
              labelId="phase-select-label"
              inputProps={{ id: 'fw-phase-select' }}
              value={phase}
              onChange={(e) => setPhase(e.target.value)}
              label="Phase"
              disabled={uploading}
            >
              {PHASES.map((p) => (
                <MenuItem key={p.value} value={p.value}>
                  {p.label}
                </MenuItem>
              ))}
            </Select>
          </FormControl>

          {/* Mode toggle */}
          <Tabs
            value={tab}
            onChange={(_, v) => {
              setTab(v);
              setError(null);
            }}
            variant="fullWidth"
          >
            <Tab label="Upload File" disabled={uploading} />
            <Tab label="External URL" disabled={uploading} />
          </Tabs>

          {/* Upload File mode */}
          {tab === 0 && (
            <Box>
              <input
                ref={fileInputRef}
                id="fw-file-input"
                type="file"
                hidden
                onChange={handleFileChange}
                autoComplete="off"
              />
              <Button
                variant="outlined"
                fullWidth
                disabled={uploading}
                onClick={() => fileInputRef.current && fileInputRef.current.click()}
                startIcon={
                  <SvgIcon fontSize="small">
                    <CloudArrowUpIcon />
                  </SvgIcon>
                }
                sx={{ py: 2 }}
              >
                {file ? file.name : 'Browse or drag a firmware file'}
              </Button>
              {file && (
                <Typography variant="caption" color="text.secondary" sx={{ mt: 1, display: 'block' }}>
                  Selected: {file.name} ({(file.size / 1024).toLocaleString(undefined, { maximumFractionDigits: 1 })} KB)
                </Typography>
              )}
            </Box>
          )}

          {/* External URL mode */}
          {tab === 1 && (
            <Stack spacing={2}>
              <TextField
                id="fw-download-url"
                label="Download URL"
                variant="outlined"
                fullWidth
                required
                value={downloadUrl}
                onChange={(e) => setDownloadUrl(e.target.value)}
                disabled={uploading}
                placeholder="https://example.com/firmware.bin"
                autoComplete="off"
              />
              <TextField
                id="fw-file-name"
                label="File Name (optional)"
                variant="outlined"
                fullWidth
                value={fileName}
                onChange={(e) => setFileName(e.target.value)}
                disabled={uploading}
                placeholder="e.g., firmware-v2.1.bin"
                autoComplete="off"
              />
              <TextField
                id="fw-fingerprint"
                label="Fingerprint (optional)"
                variant="outlined"
                fullWidth
                value={fingerprint}
                onChange={(e) => setFingerprint(e.target.value)}
                disabled={uploading}
                placeholder="e.g., sha256:abc123..."
                autoComplete="off"
                inputProps={{ style: { fontFamily: 'monospace' }, autoComplete: 'off' }}
              />
            </Stack>
          )}
        </Stack>
      </DialogContent>

      <DialogActions sx={{ px: 3, pb: 2 }}>
        <Button onClick={handleClose} disabled={uploading}>
          Cancel
        </Button>
        <Button
          onClick={handleSubmit}
          variant="contained"
          disabled={
            uploading ||
            !name.trim() ||
            !buildVersion.trim() ||
            (tab === 0 && !file) ||
            (tab === 1 && !downloadUrl.trim())
          }
          startIcon={uploading ? <CircularProgress size={16} color="inherit" /> : null}
        >
          {uploading ? 'Uploading...' : 'Upload'}
        </Button>
      </DialogActions>
    </Dialog>
  );
};

FirmwareUploadDialog.propTypes = {
  open: PropTypes.bool.isRequired,
  onClose: PropTypes.func.isRequired,
  onSuccess: PropTypes.func,
};
