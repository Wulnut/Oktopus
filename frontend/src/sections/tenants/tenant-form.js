import PropTypes from 'prop-types';
import { useState } from 'react';
import {
  Backdrop,
  Button,
  CircularProgress,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  Stack,
  TextField,
} from '@mui/material';

export const TenantForm = (props) => {
  const { open, onClose, onSubmit } = props;

  const [formData, setFormData] = useState({
    name: '',
    admin_email: '',
    admin_password: '',
  });
  const [errors, setErrors] = useState({});
  const [submitting, setSubmitting] = useState(false);

  const validateEmail = (email) => {
    return email.match(
      /^(([^<>()[\]\\.,;:\s@"]+(\.[^<>()[\]\\.,;:\s@"]+)*)|(".+"))@((\[[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\])|(([a-zA-Z\-0-9]+\.)+[a-zA-Z]{2,}))$/
    );
  };

  const handleClose = () => {
    setFormData({ name: '', admin_email: '', admin_password: '' });
    setErrors({});
    setSubmitting(false);
    onClose();
  };

  const handleSubmit = async (e) => {
    if (e) e.preventDefault();
    const newErrors = {};
    if (!formData.name.trim()) {
      newErrors.name = 'Tenant name is required';
    }
    if (!formData.admin_email.trim()) {
      newErrors.admin_email = 'Admin email is required';
    } else if (!validateEmail(formData.admin_email)) {
      newErrors.admin_email = 'Invalid email address';
    }
    if (!formData.admin_password) {
      newErrors.admin_password = 'Password is required';
    } else if (formData.admin_password.length < 8) {
      newErrors.admin_password = 'Password must be at least 8 characters';
    }

    if (Object.keys(newErrors).length > 0) {
      setErrors(newErrors);
      return;
    }

    setErrors({});
    setSubmitting(true);
    try {
      await onSubmit(formData);
      handleClose();
    } catch (e) {
      setSubmitting(false);
    }
  };

  return (
    <Dialog open={open} onClose={handleClose} maxWidth="sm" fullWidth>
      <DialogTitle>Create Tenant</DialogTitle>
      <DialogContent>
        <Stack spacing={2} sx={{ mt: 1 }}>
          <TextField
            required
            label="Tenant Name"
            value={formData.name}
            onChange={(e) =>
              setFormData({ ...formData, name: e.target.value })
            }
            error={!!errors.name}
            helperText={errors.name}
            fullWidth
          />
          <TextField
            required
            label="Admin Email"
            type="email"
            value={formData.admin_email}
            onChange={(e) =>
              setFormData({ ...formData, admin_email: e.target.value })
            }
            error={!!errors.admin_email}
            helperText={errors.admin_email}
            fullWidth
          />
          <TextField
            required
            label="Admin Password"
            type="password"
            autoComplete="new-password"
            value={formData.admin_password}
            onChange={(e) =>
              setFormData({ ...formData, admin_password: e.target.value })
            }
            error={!!errors.admin_password}
            helperText={errors.admin_password}
            fullWidth
          />
        </Stack>
      </DialogContent>
      <DialogActions>
        <Button type="button" onClick={handleClose}>Cancel</Button>
        <Button type="button" onClick={handleSubmit} variant="contained">
          Create
        </Button>
      </DialogActions>
      <Backdrop
        sx={{ color: '#fff', zIndex: (theme) => theme.zIndex.drawer + 1 }}
        open={submitting}
      >
        <CircularProgress color="inherit" />
      </Backdrop>
    </Dialog>
  );
};

TenantForm.propTypes = {
  open: PropTypes.bool.isRequired,
  onClose: PropTypes.func.isRequired,
  onSubmit: PropTypes.func.isRequired,
};
