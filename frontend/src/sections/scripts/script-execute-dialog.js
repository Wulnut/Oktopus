import { useEffect, useState } from 'react';
import {
  Box,
  Button,
  Chip,
  CircularProgress,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  Divider,
  Stack,
  Step,
  StepLabel,
  Stepper,
  TextField,
  Typography,
} from '@mui/material';
import { useBackendContext } from 'src/contexts/backend-context';
import { useAlertContext } from 'src/contexts/error-context';

export const ScriptExecuteDialog = ({ open, onClose, script }) => {
  const { httpRequest, apiPrefix } = useBackendContext();
  const { setAlert } = useAlertContext();

  const [deviceSN, setDeviceSN] = useState('');
  const [mtp, setMtp] = useState('any');
  const [variables, setVariables] = useState({});
  const [executing, setExecuting] = useState(false);
  const [result, setResult] = useState(null);

  useEffect(() => {
    if (open && script) {
      setDeviceSN('');
      setMtp('any');
      setResult(null);
      setExecuting(false);
      // Initialize variables with defaults
      const vars = {};
      (script.variables || []).forEach((v) => {
        vars[v.name] = v.default || '';
      });
      setVariables(vars);
    }
  }, [open, script]);

  const handleExecute = async () => {
    if (!deviceSN) {
      setAlert({ severity: 'error', message: 'Device serial number is required.' });
      return;
    }
    setExecuting(true);
    setResult(null);
    try {
      const { status, result: execResult } = await httpRequest(
        `${apiPrefix}/scripts/${script.id}/execute/${encodeURIComponent(deviceSN)}/${mtp}`,
        'POST',
        JSON.stringify({ variables })
      );
      if (status === 200 && execResult) {
        setResult(execResult);
      }
    } finally {
      setExecuting(false);
    }
  };

  const statusColor = (status) => {
    switch (status) {
      case 'completed': return 'success';
      case 'failed': return 'error';
      case 'running': return 'warning';
      default: return 'default';
    }
  };

  return (
    <Dialog open={open} onClose={executing ? undefined : onClose} maxWidth="md" fullWidth scroll="paper">
      <DialogTitle>Execute: {script?.name}</DialogTitle>
      <DialogContent dividers>
        <Stack spacing={3}>
          {/* Device selection */}
          <Stack direction="row" spacing={2}>
            <TextField
              label="Device Serial Number"
              value={deviceSN}
              onChange={(e) => setDeviceSN(e.target.value)}
              fullWidth
              required
              disabled={executing}
              id="exec-device-sn"
              autoComplete="off"
            />
            <TextField
              label="MTP"
              select
              value={mtp}
              onChange={(e) => setMtp(e.target.value)}
              sx={{ minWidth: 120 }}
              SelectProps={{ native: true }}
              disabled={executing}
              id="exec-mtp"
            >
              <option value="any">Auto</option>
              <option value="mqtt">MQTT</option>
              <option value="ws">WebSocket</option>
              <option value="stomp">STOMP</option>
            </TextField>
          </Stack>

          {/* Variables */}
          {(script?.variables || []).length > 0 && (
            <>
              <Divider />
              <Typography variant="subtitle2">Variables</Typography>
              {script.variables.map((v) => (
                <TextField
                  key={v.name}
                  label={`${v.name}${v.required ? ' *' : ''}`}
                  helperText={v.description}
                  value={variables[v.name] || ''}
                  onChange={(e) => setVariables((prev) => ({ ...prev, [v.name]: e.target.value }))}
                  fullWidth
                  size="small"
                  disabled={executing}
                  id={`exec-var-${v.name}`}
                  autoComplete="off"
                />
              ))}
            </>
          )}

          {/* Results */}
          {result && (
            <>
              <Divider />
              <Stack direction="row" spacing={2} alignItems="center">
                <Typography variant="subtitle1">Result</Typography>
                <Chip label={result.status} color={statusColor(result.status)} size="small" />
                {result.started_at && result.finished_at && (
                  <Typography variant="caption" color="text.secondary">
                    {((new Date(result.finished_at) - new Date(result.started_at)) / 1000).toFixed(1)}s
                  </Typography>
                )}
              </Stack>

              <Stepper orientation="vertical" activeStep={-1}>
                {(result.step_results || []).map((sr, i) => (
                  <Step key={i} completed={sr.status === 'success'}>
                    <StepLabel
                      error={sr.status === 'failed'}
                      optional={
                        <Stack spacing={0.5}>
                          {sr.error && (
                            <Typography variant="caption" color="error">{sr.error}</Typography>
                          )}
                          {sr.condition_result !== undefined && sr.condition_result !== null && (
                            <Typography variant="caption">
                              Condition: {sr.condition_result ? 'true' : 'false'}
                              {sr.jumped_to ? ` → ${sr.jumped_to}` : ''}
                            </Typography>
                          )}
                          {sr.response && (
                            <Box
                              component="pre"
                              sx={{
                                mt: 0.5,
                                p: 1,
                                bgcolor: 'grey.50',
                                borderRadius: 1,
                                fontSize: 11,
                                maxHeight: 200,
                                overflow: 'auto',
                                whiteSpace: 'pre-wrap',
                                wordBreak: 'break-all',
                              }}
                            >
                              {JSON.stringify(sr.response, null, 2)}
                            </Box>
                          )}
                        </Stack>
                      }
                    >
                      <Stack direction="row" spacing={1} alignItems="center">
                        <Typography variant="body2">
                          {sr.step_name || sr.step_id}
                        </Typography>
                        <Chip
                          label={sr.status}
                          size="small"
                          color={sr.status === 'success' ? 'success' : 'error'}
                          variant="outlined"
                        />
                      </Stack>
                    </StepLabel>
                  </Step>
                ))}
              </Stepper>
            </>
          )}
        </Stack>
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose} disabled={executing}>Close</Button>
        <Button
          onClick={handleExecute}
          variant="contained"
          disabled={executing || !deviceSN}
          startIcon={executing ? <CircularProgress size={16} /> : null}
        >
          {executing ? 'Executing...' : result ? 'Run Again' : 'Execute'}
        </Button>
      </DialogActions>
    </Dialog>
  );
};
