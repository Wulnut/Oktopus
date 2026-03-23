import { useCallback, useEffect, useState } from 'react';
import Head from 'next/head';
import {
  Box,
  Button,
  Card,
  CardContent,
  CardHeader,
  Chip,
  CircularProgress,
  Collapse,
  Container,
  Divider,
  FormControl,
  IconButton,
  InputLabel,
  MenuItem,
  Select,
  Slider,
  Stack,
  SvgIcon,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableRow,
  TextField,
  Typography,
} from '@mui/material';
import PlayIcon from '@heroicons/react/24/outline/PlayIcon';
import ChevronDownIcon from '@heroicons/react/24/outline/ChevronDownIcon';
import ChevronUpIcon from '@heroicons/react/24/outline/ChevronUpIcon';
import { Layout as DashboardLayout } from 'src/layouts/dashboard/layout';
import { DeviceSelector } from 'src/sections/mass-actions/device-selector';
import { MassActionDetail } from 'src/sections/mass-actions/mass-action-detail';
import { useBackendContext } from 'src/contexts/backend-context';
import { useAlertContext } from 'src/contexts/error-context';

const Page = () => {
  const { httpRequest } = useBackendContext();
  const { setAlert } = useAlertContext();

  // Script selection
  const [scripts, setScripts] = useState([]);
  const [scriptsLoading, setScriptsLoading] = useState(true);
  const [selectedScriptId, setSelectedScriptId] = useState('');

  // Variables
  const [variables, setVariables] = useState({});

  // Device selection
  const [selectedSNs, setSelectedSNs] = useState([]);

  // Concurrency
  const [concurrency, setConcurrency] = useState(5);

  // Submit
  const [submitting, setSubmitting] = useState(false);

  // Recent jobs
  const [jobs, setJobs] = useState([]);
  const [jobsLoading, setJobsLoading] = useState(true);
  const [expandedJob, setExpandedJob] = useState(null);

  const fetchScripts = useCallback(async () => {
    setScriptsLoading(true);
    try {
      const { status, result } = await httpRequest('/api/scripts', 'GET');
      if (status === 200 && Array.isArray(result)) setScripts(result);
    } finally {
      setScriptsLoading(false);
    }
  }, []);

  const fetchJobs = useCallback(async () => {
    setJobsLoading(true);
    try {
      const { status, result } = await httpRequest('/api/mass-actions', 'GET');
      if (status === 200 && Array.isArray(result)) {
        setJobs(result.filter((j) => j.type === 'script'));
      }
    } finally {
      setJobsLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchScripts();
    fetchJobs();
  }, [fetchScripts, fetchJobs]);

  // Auto-refresh jobs while any is running
  useEffect(() => {
    const hasRunning = jobs.some((j) => j.status === 'running');
    if (!hasRunning) return;
    const interval = setInterval(fetchJobs, 5000);
    return () => clearInterval(interval);
  }, [jobs]);

  const selectedScript = scripts.find((s) => s.id === selectedScriptId);

  // Initialize variables when script changes
  useEffect(() => {
    if (!selectedScript) {
      setVariables({});
      return;
    }
    const vars = {};
    for (const v of selectedScript.variables || []) {
      vars[v.name] = v.default || '';
    }
    setVariables(vars);
  }, [selectedScriptId]);

  const handleSubmit = async () => {
    if (!selectedScript || selectedSNs.length === 0) return;
    setSubmitting(true);
    try {
      const body = JSON.stringify({
        script_id: selectedScript.id,
        variables,
        device_sns: selectedSNs,
        concurrency,
      });
      const { status } = await httpRequest('/api/mass-actions/script', 'POST', body);
      if (status === 202 || status === 200) {
        setAlert({ severity: 'success', message: 'Mass script execution started.' });
        setSelectedSNs([]);
        fetchJobs();
      }
    } finally {
      setSubmitting(false);
    }
  };

  const handleCancel = async (id) => {
    const { status } = await httpRequest(`/api/mass-actions/${id}/cancel`, 'POST');
    if (status === 204 || status === 200) {
      setAlert({ severity: 'info', message: 'Mass action cancelled.' });
      fetchJobs();
    }
  };

  return (
    <>
      <Head>
        <title>Mass Script Execution | Oktopus</title>
      </Head>
      <Box component="main" sx={{ flexGrow: 1, py: 8 }}>
        <Container maxWidth="xl">
          <Stack spacing={3}>
            <Typography variant="h4">Mass Script Execution</Typography>

            {/* Card 1 — Run Script */}
            <Card>
              <CardHeader title="Run Script" />
              <Divider />
              <CardContent>
                <Stack spacing={3}>
                  {/* Script selector */}
                  <Typography variant="subtitle2">1. Select Script</Typography>
                  {scriptsLoading ? (
                    <Box display="flex" justifyContent="center" py={2}>
                      <CircularProgress />
                    </Box>
                  ) : scripts.length === 0 ? (
                    <Typography color="text.secondary" variant="body2">
                      No scripts available. Create a script first.
                    </Typography>
                  ) : (
                    <FormControl fullWidth>
                      <InputLabel id="script-select-label">Script</InputLabel>
                      <Select
                        labelId="script-select-label"
                        value={selectedScriptId}
                        onChange={(e) => {
                          setSelectedScriptId(e.target.value);
                          setSelectedSNs([]);
                        }}
                        label="Script"
                      >
                        {scripts.map((s) => (
                          <MenuItem key={s.id} value={s.id}>
                            <Stack direction="row" spacing={1} alignItems="center">
                              <span>{s.name}</span>
                              <Typography variant="caption" color="text.secondary">
                                ({(s.steps || []).length} steps)
                              </Typography>
                            </Stack>
                          </MenuItem>
                        ))}
                      </Select>
                    </FormControl>
                  )}

                  {/* Variable inputs */}
                  {selectedScript && (selectedScript.variables || []).length > 0 && (
                    <>
                      <Divider />
                      <Typography variant="subtitle2">2. Variables</Typography>
                      <Stack spacing={2}>
                        {selectedScript.variables.map((v) => (
                          <TextField
                            key={v.name}
                            label={v.name}
                            value={variables[v.name] || ''}
                            onChange={(e) =>
                              setVariables((prev) => ({ ...prev, [v.name]: e.target.value }))
                            }
                            fullWidth
                            required={v.required}
                            helperText={v.description}
                            placeholder={v.default || ''}
                            autoComplete="off"
                          />
                        ))}
                      </Stack>
                    </>
                  )}

                  {/* Device selector */}
                  {selectedScript && (
                    <>
                      <Divider />
                      <Typography variant="subtitle2">
                        {(selectedScript.variables || []).length > 0 ? '3' : '2'}. Select Devices
                      </Typography>
                      <DeviceSelector
                        selectedSNs={selectedSNs}
                        onChange={setSelectedSNs}
                      />
                    </>
                  )}

                  {/* Concurrency */}
                  {selectedScript && selectedSNs.length > 0 && (
                    <>
                      <Divider />
                      <Typography variant="subtitle2">Concurrency</Typography>
                      <Box sx={{ px: 2, maxWidth: 400 }}>
                        <Slider
                          value={concurrency}
                          onChange={(_, v) => setConcurrency(v)}
                          min={1}
                          max={20}
                          step={1}
                          marks={[
                            { value: 1, label: '1' },
                            { value: 5, label: '5' },
                            { value: 10, label: '10' },
                            { value: 20, label: '20' },
                          ]}
                          valueLabelDisplay="auto"
                        />
                      </Box>
                    </>
                  )}

                  {/* Submit */}
                  {selectedScript && selectedSNs.length > 0 && (
                    <>
                      <Divider />
                      <Stack direction="row" spacing={2} alignItems="center">
                        <Button
                          variant="contained"
                          color="success"
                          onClick={handleSubmit}
                          disabled={submitting}
                          startIcon={
                            submitting ? (
                              <CircularProgress size={16} color="inherit" />
                            ) : (
                              <SvgIcon fontSize="small">
                                <PlayIcon />
                              </SvgIcon>
                            )
                          }
                        >
                          Execute ({selectedSNs.length} devices)
                        </Button>
                        <Typography variant="body2" color="text.secondary">
                          Script: {selectedScript.name} &middot; Concurrency: {concurrency}
                        </Typography>
                      </Stack>
                    </>
                  )}
                </Stack>
              </CardContent>
            </Card>

            {/* Card 2 — Recent Jobs */}
            <Card>
              <CardHeader title="Recent Jobs" />
              <Divider />
              <CardContent sx={{ p: 0 }}>
                {jobsLoading ? (
                  <Box display="flex" justifyContent="center" py={4}>
                    <CircularProgress />
                  </Box>
                ) : jobs.length === 0 ? (
                  <Box display="flex" justifyContent="center" py={4}>
                    <Typography color="text.secondary" variant="body2">
                      No script execution jobs yet.
                    </Typography>
                  </Box>
                ) : (
                  <Table size="small">
                    <TableHead>
                      <TableRow>
                        <TableCell sx={{ fontWeight: 700 }} />
                        <TableCell sx={{ fontWeight: 700 }}>Name</TableCell>
                        <TableCell sx={{ fontWeight: 700 }}>Script</TableCell>
                        <TableCell sx={{ fontWeight: 700 }}>Progress</TableCell>
                        <TableCell sx={{ fontWeight: 700 }}>Status</TableCell>
                        <TableCell sx={{ fontWeight: 700 }}>Created</TableCell>
                      </TableRow>
                    </TableHead>
                    <TableBody>
                      {jobs.map((job) => (
                        <>
                          <TableRow
                            key={job.id}
                            hover
                            onClick={() => setExpandedJob(expandedJob === job.id ? null : job.id)}
                            sx={{ cursor: 'pointer' }}
                          >
                            <TableCell sx={{ width: 40 }}>
                              <IconButton size="small">
                                <SvgIcon fontSize="small">
                                  {expandedJob === job.id ? <ChevronUpIcon /> : <ChevronDownIcon />}
                                </SvgIcon>
                              </IconButton>
                            </TableCell>
                            <TableCell sx={{ fontWeight: 500 }}>{job.name}</TableCell>
                            <TableCell>{job.script_name || '—'}</TableCell>
                            <TableCell>
                              {job.progress}/{job.total_devices}
                            </TableCell>
                            <TableCell>
                              <Chip
                                label={job.status}
                                size="small"
                                color={
                                  job.status === 'completed'
                                    ? 'success'
                                    : job.status === 'running'
                                    ? 'info'
                                    : job.status === 'failed'
                                    ? 'error'
                                    : 'default'
                                }
                                variant="outlined"
                              />
                            </TableCell>
                            <TableCell>
                              {job.created_at
                                ? new Date(job.created_at).toLocaleString()
                                : '—'}
                            </TableCell>
                          </TableRow>
                          <TableRow key={`${job.id}-detail`}>
                            <TableCell colSpan={6} sx={{ py: 0, px: 0 }}>
                              <Collapse in={expandedJob === job.id} unmountOnExit>
                                <MassActionDetail actionId={job.id} onCancel={handleCancel} />
                              </Collapse>
                            </TableCell>
                          </TableRow>
                        </>
                      ))}
                    </TableBody>
                  </Table>
                )}
              </CardContent>
            </Card>
          </Stack>
        </Container>
      </Box>
    </>
  );
};

Page.getLayout = (page) => <DashboardLayout>{page}</DashboardLayout>;

export default Page;
