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
  IconButton,
  Radio,
  Slider,
  Stack,
  SvgIcon,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Paper,
  Typography,
} from '@mui/material';
import ArrowDownTrayIcon from '@heroicons/react/24/outline/ArrowDownTrayIcon';
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

  // Firmware selection
  const [firmware, setFirmware] = useState([]);
  const [fwLoading, setFwLoading] = useState(true);
  const [selectedFwId, setSelectedFwId] = useState('');

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

  const fetchFirmware = useCallback(async () => {
    setFwLoading(true);
    try {
      const { status, result } = await httpRequest('/api/firmware', 'GET');
      if (status === 200 && Array.isArray(result)) setFirmware(result);
    } finally {
      setFwLoading(false);
    }
  }, []);

  const fetchJobs = useCallback(async () => {
    setJobsLoading(true);
    try {
      const { status, result } = await httpRequest('/api/mass-actions', 'GET');
      if (status === 200 && Array.isArray(result)) {
        setJobs(result.filter((j) => j.type === 'firmware_update'));
      }
    } finally {
      setJobsLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchFirmware();
    fetchJobs();
  }, [fetchFirmware, fetchJobs]);

  // Auto-refresh jobs while any is running
  useEffect(() => {
    const hasRunning = jobs.some((j) => j.status === 'running');
    if (!hasRunning) return;
    const interval = setInterval(fetchJobs, 5000);
    return () => clearInterval(interval);
  }, [jobs]);

  const selectedFw = firmware.find((fw) => fw.id === selectedFwId);

  const handleSubmit = async () => {
    if (!selectedFw || selectedSNs.length === 0) return;
    setSubmitting(true);
    try {
      const body = JSON.stringify({
        firmware_id: selectedFw.id,
        device_sns: selectedSNs,
        concurrency,
      });
      const { status } = await httpRequest('/api/mass-actions/firmware', 'POST', body);
      if (status === 202 || status === 200) {
        setAlert({ severity: 'success', message: 'Mass firmware update started.' });
        setSelectedSNs([]);
        setSelectedFwId('');
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
        <title>Mass Firmware Update | Oktopus</title>
      </Head>
      <Box component="main" sx={{ flexGrow: 1, py: 8 }}>
        <Container maxWidth="xl">
          <Stack spacing={3}>
            <Typography variant="h4">Mass Firmware Update</Typography>

            {/* Card 1 — New Update */}
            <Card>
              <CardHeader title="New Firmware Update" />
              <Divider />
              <CardContent>
                <Stack spacing={3}>
                  {/* Firmware selector */}
                  <Typography variant="subtitle2">1. Select Firmware</Typography>
                  {fwLoading ? (
                    <Box display="flex" justifyContent="center" py={2}>
                      <CircularProgress />
                    </Box>
                  ) : firmware.length === 0 ? (
                    <Typography color="text.secondary" variant="body2">
                      No firmware available. Upload firmware first.
                    </Typography>
                  ) : (
                    <TableContainer component={Paper} variant="outlined" sx={{ maxHeight: 300 }}>
                      <Table size="small" stickyHeader>
                        <TableHead>
                          <TableRow>
                            <TableCell padding="checkbox" />
                            <TableCell sx={{ fontWeight: 700 }}>Name</TableCell>
                            <TableCell sx={{ fontWeight: 700 }}>Version</TableCell>
                            <TableCell sx={{ fontWeight: 700 }}>Vendor</TableCell>
                            <TableCell sx={{ fontWeight: 700 }}>Model</TableCell>
                            <TableCell sx={{ fontWeight: 700 }}>Phase</TableCell>
                          </TableRow>
                        </TableHead>
                        <TableBody>
                          {firmware.map((fw) => (
                            <TableRow
                              key={fw.id}
                              hover
                              selected={selectedFwId === fw.id}
                              onClick={() => {
                                setSelectedFwId(fw.id);
                                setSelectedSNs([]);
                              }}
                              sx={{ cursor: 'pointer' }}
                            >
                              <TableCell padding="checkbox">
                                <Radio
                                  size="small"
                                  checked={selectedFwId === fw.id}
                                  onChange={() => {
                                    setSelectedFwId(fw.id);
                                    setSelectedSNs([]);
                                  }}
                                />
                              </TableCell>
                              <TableCell sx={{ fontWeight: 600 }}>{fw.name}</TableCell>
                              <TableCell>{fw.build_version || '—'}</TableCell>
                              <TableCell>{fw.vendor || '—'}</TableCell>
                              <TableCell>{fw.model || '—'}</TableCell>
                              <TableCell>
                                <Chip
                                  label={fw.phase === 'release' ? 'Release' : 'Internal Testing'}
                                  size="small"
                                  color={fw.phase === 'release' ? 'success' : 'info'}
                                  variant="outlined"
                                />
                              </TableCell>
                            </TableRow>
                          ))}
                        </TableBody>
                      </Table>
                    </TableContainer>
                  )}

                  {/* Device selector */}
                  {selectedFw && (
                    <>
                      <Divider />
                      <Typography variant="subtitle2">2. Select Devices</Typography>
                      <DeviceSelector
                        vendor={selectedFw.vendor}
                        model={selectedFw.model}
                        selectedSNs={selectedSNs}
                        onChange={setSelectedSNs}
                      />
                    </>
                  )}

                  {/* Concurrency */}
                  {selectedFw && selectedSNs.length > 0 && (
                    <>
                      <Divider />
                      <Typography variant="subtitle2">3. Concurrency</Typography>
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
                  {selectedFw && selectedSNs.length > 0 && (
                    <>
                      <Divider />
                      <Stack direction="row" spacing={2} alignItems="center">
                        <Button
                          variant="contained"
                          color="primary"
                          onClick={handleSubmit}
                          disabled={submitting}
                          startIcon={
                            submitting ? (
                              <CircularProgress size={16} color="inherit" />
                            ) : (
                              <SvgIcon fontSize="small">
                                <ArrowDownTrayIcon />
                              </SvgIcon>
                            )
                          }
                        >
                          Start Update ({selectedSNs.length} devices)
                        </Button>
                        <Typography variant="body2" color="text.secondary">
                          Firmware: {selectedFw.name} v{selectedFw.build_version} &middot; Concurrency: {concurrency}
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
                      No firmware update jobs yet.
                    </Typography>
                  </Box>
                ) : (
                  <Table size="small">
                    <TableHead>
                      <TableRow>
                        <TableCell sx={{ fontWeight: 700 }} />
                        <TableCell sx={{ fontWeight: 700 }}>Name</TableCell>
                        <TableCell sx={{ fontWeight: 700 }}>Firmware</TableCell>
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
                            <TableCell>{job.firmware_name || '—'}</TableCell>
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
