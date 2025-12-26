import { useState, useEffect } from 'react';
import {
  Card,
  CardContent,
  CardHeader,
  CardActions,
  Button,
  TextField,
  Stack,
  Divider,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogContentText,
  DialogActions,
  Box,
  IconButton,
  SvgIcon,
  Alert,
  Typography,
  CircularProgress,
  Backdrop,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Paper,
  Chip,
  FormControlLabel,
  Checkbox,
  Select,
  MenuItem,
  InputLabel,
  FormControl,
} from '@mui/material';
import { useRouter } from 'next/router';
import { useBackendContext } from 'src/contexts/backend-context';
import XMarkIcon from '@heroicons/react/24/outline/XMarkIcon';
import PlusCircleIcon from '@heroicons/react/24/outline/PlusCircleIcon';
import TrashIcon from '@heroicons/react/24/outline/TrashIcon';
import ArrowPathIcon from '@heroicons/react/24/outline/ArrowPathIcon';

// Generate UUID v5
// Pattern: xxxxxxxx-xxxx-5xxx-Nxxx-xxxxxxxxxxxx
// Position 14 (0-indexed) = '5' (version)
// Position 19 (N) = 8, 9, a, or b (variant)
const generateUUID = () => {
  return 'xxxxxxxx-xxxx-5xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, function(c) {
    const r = Math.random() * 16 | 0;
    if (c === 'x') {
      return r.toString(16);
    } else {
      // Variant bits: must be 8, 9, a, or b
      const v = (r & 0x3 | 0x8);
      return v.toString(16);
    }
  });
};

// Format time ago
const formatTimeAgo = (timestamp) => {
  if (!timestamp) return 'Unknown';
  
  const now = new Date();
  const then = new Date(timestamp);
  const diffMs = now - then;
  const diffMins = Math.floor(diffMs / 60000);
  const diffHours = Math.floor(diffMs / 3600000);
  const diffDays = Math.floor(diffMs / 86400000);
  
  if (diffDays > 0) {
    const hours = Math.floor((diffMs % 86400000) / 3600000);
    return `${diffDays} day${diffDays > 1 ? 's' : ''} ${hours} hour${hours !== 1 ? 's' : ''} ago`;
  } else if (diffHours > 0) {
    const mins = Math.floor((diffMs % 3600000) / 60000);
    return `${diffHours} hour${diffHours > 1 ? 's' : ''} ${mins} minute${mins !== 1 ? 's' : ''} ago`;
  } else {
    return `${diffMins} minute${diffMins !== 1 ? 's' : ''} ago`;
  }
};

export const DevicesLCM = () => {
  const router = useRouter();
  const { httpRequest } = useBackendContext();
  const deviceID = router.query.id[0];

  const [deploymentUnits, setDeploymentUnits] = useState([]);
  const [executionEnvironments, setExecutionEnvironments] = useState([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(null);
  const [response, setResponse] = useState(null);
  
  // Install dialog state
  const [showInstallDialog, setShowInstallDialog] = useState(false);
  const [installUrl, setInstallUrl] = useState('');
  const [installUuid, setInstallUuid] = useState(generateUUID());
  const [installExecEnv, setInstallExecEnv] = useState('Device.SoftwareModules.ExecEnv.1.');
  const [installPrivileged, setInstallPrivileged] = useState(true);
  const [dockerRegistryUrl, setDockerRegistryUrl] = useState('');
  const [dockerImages, setDockerImages] = useState([]);
  const [loadingDockerImages, setLoadingDockerImages] = useState(false);
  
  // Uninstall dialog state
  const [showUninstallDialog, setShowUninstallDialog] = useState(false);
  const [uninstallTarget, setUninstallTarget] = useState(null);

  // Fetch Deployment Units list
  const fetchDeploymentUnits = async () => {
    setLoading(true);
    setError(null);

    try {
      const getCommand = {
        header: {
          msg_id: generateUUID(),
          msg_type: 1, // GET
        },
        body: {
          request: {
            get: {
              paramPaths: [
                'Device.SoftwareModules.DeploymentUnit.*.',
              ],
              maxDepth: 2,
            },
          },
        },
      };

      const { result, status } = await httpRequest(
        `/api/device/${deviceID}/any/generic`,
        'PUT',
        JSON.stringify(getCommand),
        null
      );

      if (status === 200 && result) {
        // Parse the response to extract DeploymentUnit data
        // This depends on the actual response structure
        parseDeploymentUnits(result);
      } else {
        setError('Failed to fetch deployment units');
      }
    } catch (err) {
      setError(err.message || 'An error occurred while fetching deployment units');
    } finally {
      setLoading(false);
    }
  };

  // Parse DeploymentUnits from GET response
  const parseDeploymentUnits = (response) => {
    const units = [];
    
    if (response.req_path_results) {
      response.req_path_results.forEach(pathResult => {
        if (pathResult.resolved_path_results) {
          pathResult.resolved_path_results.forEach(resolved => {
            const resolvedPath = resolved.resolved_path || '';
            
            // Only process paths that are actually DeploymentUnit instances
            if (!resolvedPath.includes('Device.SoftwareModules.DeploymentUnit.')) {
              return;
            }
            
            const pathParts = resolvedPath.split('.');
            const instanceIndex = pathParts[pathParts.length - 2];
            
            // Validate instance index is a number
            if (!instanceIndex || isNaN(instanceIndex)) {
              return;
            }
            
            if (resolved.result_params) {
              const params = resolved.result_params;
              
              // Only include if it has a valid Name (not empty, not "generic")
              const name = params.Name || params.Alias || '';
              if (!name || name === 'generic' || name.trim() === '') {
                return;
              }
              
              units.push({
                instance: instanceIndex,
                name: name,
                version: params.Version || params.ModuleVersion || 'Unknown',
                status: params.Status || 'Unknown',
                url: params.URL || '',
                execEnvRef: params.ExecutionEnvRef || '',
                resolved: params.Resolved === 'true' || params.Resolved === true,
                installedTime: params.Installed || null,
                alias: params.Alias || '',
                duid: params.DUID || '',
                path: resolvedPath,
              });
            }
          });
        }
      });
    }
    
    setDeploymentUnits(units);
  };

  // Fetch Execution Environments list
  const fetchExecutionEnvironments = async () => {
    try {
      const getCommand = {
        header: {
          msg_id: generateUUID(),
          msg_type: 1, // GET
        },
        body: {
          request: {
            get: {
              paramPaths: [
                'Device.SoftwareModules.ExecEnv.*.',
              ],
              maxDepth: 1,
            },
          },
        },
      };

      const { result, status } = await httpRequest(
        `/api/device/${deviceID}/any/generic`,
        'PUT',
        JSON.stringify(getCommand),
        null
      );

      if (status === 200 && result) {
        parseExecutionEnvironments(result);
      }
    } catch (err) {
      console.error('Failed to fetch execution environments:', err);
      // Set default if fetch fails
      setExecutionEnvironments(['Device.SoftwareModules.ExecEnv.1.']);
    }
  };

  // Parse ExecutionEnvironments from GET response
  const parseExecutionEnvironments = (response) => {
    const envs = ['Device.SoftwareModules.ExecEnv.1.']; // Default
    
    if (response.req_path_results) {
      response.req_path_results.forEach(pathResult => {
        if (pathResult.resolved_path_results) {
          pathResult.resolved_path_results.forEach(resolved => {
            const resolvedPath = resolved.resolved_path || '';
            const match = resolvedPath.match(/ExecEnv\.(\d+)\./);
            if (match) {
              const instanceIndex = match[1];
              const envPath = `Device.SoftwareModules.ExecEnv.${instanceIndex}.`;
              if (!envs.includes(envPath)) {
                envs.push(envPath);
              }
            }
          });
        }
      });
    }
    
    setExecutionEnvironments(envs);
    if (envs.length > 0) {
      setInstallExecEnv(envs[0]);
    }
  };

  // Fetch Docker images from registry
  // Note: This requires a backend endpoint due to CORS and TLS certificate handling
  // Suggestion: Create /api/docker-registry/catalog endpoint in backend
  const fetchDockerImages = async () => {
    if (!dockerRegistryUrl.trim()) {
      setError('Docker registry URL is required');
      return;
    }

    setLoadingDockerImages(true);
    setError(null);
    
    try {
      // Option 1: Backend endpoint (recommended for security and CORS)
      // This would handle TLS certificates and authentication server-side
      const { result, status } = await httpRequest(
        `/api/docker-registry/catalog`,
        'POST',
        JSON.stringify({ registry_url: dockerRegistryUrl }),
        null
      );

      if (status === 200 && result?.repositories) {
        // Parse repositories and fetch tags for each
        const images = [];
        for (const repo of result.repositories) {
          const tagsResult = await httpRequest(
            `/api/docker-registry/tags`,
            'POST',
            JSON.stringify({ registry_url: dockerRegistryUrl, repository: repo }),
            null
          );
          
          if (tagsResult.status === 200 && tagsResult.result?.tags) {
            tagsResult.result.tags.forEach(tag => {
              images.push({
                name: repo,
                tag: tag,
                fullUrl: `docker://${dockerRegistryUrl.replace(/^https?:\/\//, '')}/${repo}:${tag}`,
              });
            });
          }
        }
        setDockerImages(images);
      } else {
        setError('Failed to fetch Docker images. Backend endpoint may not be implemented yet.');
      }
    } catch (err) {
      // Fallback: Show manual entry message
      setError('Docker registry API integration requires backend support. Please enter URL manually in format: docker://host/image:tag');
      console.error('Docker registry fetch error:', err);
    } finally {
      setLoadingDockerImages(false);
    }
  };

  // Ensure subscription exists for async operation
  // Returns true if subscription exists or was created successfully, false on error
  const ensureSubscription = async (commandPath) => {
    // Step 1: Check if subscription already exists
    const searchPath = `Device.LocalAgent.Subscription.[Enable=="True"&&NotifType=="OperationComplete"&&ReferenceList=="${commandPath}"].`;
    
    const checkSubscriptionCommand = {
      header: {
        msg_id: generateUUID(),
        msg_type: 1, // GET
      },
      body: {
        request: {
          get: {
            paramPaths: [searchPath],
            maxDepth: 1,
          },
        },
      },
    };

    const { result: checkResult, status: checkStatus } = await httpRequest(
      `/api/device/${deviceID}/any/generic`,
      'PUT',
      JSON.stringify(checkSubscriptionCommand),
      null
    );

    let subscriptionExists = false;
    if (checkStatus === 200 && checkResult?.req_path_results) {
      // Check if any subscriptions were found
      checkResult.req_path_results.forEach(pathResult => {
        if (pathResult.resolved_path_results && pathResult.resolved_path_results.length > 0) {
          subscriptionExists = true;
        }
      });
    }

    // Step 2: Create subscription only if it doesn't exist
    if (!subscriptionExists) {
      const subscriptionAddCommand = {
        header: {
          msg_id: generateUUID(),
          msg_type: 8, // ADD
        },
        body: {
          request: {
            add: {
              allow_partial: true,
              create_objs: [
                {
                  obj_path: 'Device.LocalAgent.Subscription.',
                  param_settings: [
                    {
                      param: 'Enable',
                      value: 'true',
                      required: true,
                    },
                    {
                      param: 'NotifType',
                      value: 'OperationComplete',
                      required: true,
                    },
                    {
                      param: 'ReferenceList',
                      value: commandPath,
                      required: true,
                    },
                  ],
                },
              ],
            },
          },
        },
      };

      const { result: addResult, status: addStatus } = await httpRequest(
        `/api/device/${deviceID}/any/generic`,
        'PUT',
        JSON.stringify(subscriptionAddCommand),
        null
      );

      if (addStatus !== 200) {
        setError(addResult?.message || addResult || 'Failed to create subscription for operation');
        return false;
      }

      // Verify subscription was created successfully
      if (addResult?.created_obj_results?.[0]?.oper_status?.OperStatus?.OperSuccess === undefined) {
        const errorMsg = addResult?.created_obj_results?.[0]?.oper_status?.OperStatus?.OperFailure?.err_msg || 
                        'Failed to create subscription';
        setError(errorMsg);
        return false;
      }
    }

    return true;
  };

  // Handle Install
  const handleInstall = async () => {
    if (!installUrl.trim()) {
      setError('URL is required');
      return;
    }

    setLoading(true);
    setError(null);
    setResponse(null);

    try {
      const installCommandPath = 'Device.SoftwareModules.InstallDU()';

      // Step 1: Ensure subscription exists
      const subscriptionReady = await ensureSubscription(installCommandPath);
      if (!subscriptionReady) {
        return;
      }

      // Step 2: Send Install command
      const installCommand = {
        header: {
          msg_id: generateUUID(),
          msg_type: 6, // OPERATE
        },
        body: {
          request: {
            operate: {
              command: installCommandPath,
              command_key: 'InstallDU',
              send_resp: true,
              input_args: {
                URL: installUrl,
                UUID: installUuid,
                ExecutionEnvRef: installExecEnv,
                Privileged: installPrivileged.toString(),
              },
            },
          },
        },
      };

      const { result, status } = await httpRequest(
        `/api/device/${deviceID}/any/generic`,
        'PUT',
        JSON.stringify(installCommand),
        null
      );

      if (status === 200) {
        const operationResult = result?.operation_results?.[0];
        const operationResp = operationResult?.OperationResp;
        
        // Check for OperSuccess
        if (operationResp?.OperSuccess !== undefined) {
          setResponse('Software module installed successfully');
          setError(null);
          setShowInstallDialog(false);
          setInstallUrl('');
          setInstallUuid(generateUUID());
          // Refresh the list immediately
          await fetchDeploymentUnits();
        } 
        // Check for CmdFailure
        else if (operationResp?.CmdFailure) {
          const errorMsg = operationResp.CmdFailure.err_msg || 
                          `Install failed: ${operationResp.CmdFailure.err_code || 'Unknown error'}`;
          setError(errorMsg);
          setResponse(null);
        } 
        // Fallback: check if there's any indication of success
        else if (!operationResp?.CmdFailure) {
          setResponse('Software module installation initiated successfully');
          setError(null);
          setShowInstallDialog(false);
          setInstallUrl('');
          setInstallUuid(generateUUID());
          // Refresh the list
          await fetchDeploymentUnits();
        } else {
          setError('Unknown response format from install operation');
          setResponse(null);
        }
      } else {
        setError(result?.message || result || 'Failed to install software module');
        setResponse(null);
      }
    } catch (err) {
      setError(err.message || 'An error occurred while installing software module');
      setResponse(null);
    } finally {
      setLoading(false);
    }
  };

  // Handle Uninstall
  const handleUninstall = async () => {
    if (!uninstallTarget) {
      return;
    }

    setLoading(true);
    setError(null);
    setResponse(null);

    try {
      const instanceNum = uninstallTarget.instance;
      const uninstallCommandPath = `Device.SoftwareModules.DeploymentUnit.${instanceNum}.Uninstall()`;

      // Step 1: Ensure subscription exists
      const subscriptionReady = await ensureSubscription(uninstallCommandPath);
      if (!subscriptionReady) {
        return;
      }

      // Step 2: Send Uninstall command with command_key
      const uninstallCommand = {
        header: {
          msg_id: generateUUID(),
          msg_type: 6, // OPERATE
        },
        body: {
          request: {
            operate: {
              command: uninstallCommandPath,
              command_key: 'Uninstall()',
              send_resp: true,
            },
          },
        },
      };

      const { result, status } = await httpRequest(
        `/api/device/${deviceID}/any/generic`,
        'PUT',
        JSON.stringify(uninstallCommand),
        null
      );

      if (status === 200) {
        const operationResult = result?.operation_results?.[0];
        const operationResp = operationResult?.OperationResp;
        
        // Check for OperSuccess
        if (operationResp?.OperSuccess !== undefined) {
          setResponse('Software module uninstalled successfully');
          setError(null);
          setShowUninstallDialog(false);
          setUninstallTarget(null);
          // Refresh the list immediately
          await fetchDeploymentUnits();
        } 
        // Check for CmdFailure
        else if (operationResp?.CmdFailure) {
          const errorMsg = operationResp.CmdFailure.err_msg || 
                          `Uninstall failed: ${operationResp.CmdFailure.err_code || 'Unknown error'}`;
          setError(errorMsg);
          setResponse(null);
        } 
        // Fallback: check if there's any indication of success
        else if (!operationResp?.CmdFailure) {
          setResponse('Software module uninstallation initiated successfully');
          setError(null);
          setShowUninstallDialog(false);
          setUninstallTarget(null);
          // Refresh the list
          await fetchDeploymentUnits();
        } else {
          setError('Unknown response format from uninstall operation');
          setResponse(null);
        }
      } else {
        setError(result?.message || result || 'Failed to uninstall software module');
        setResponse(null);
      }
    } catch (err) {
      setError(err.message || 'An error occurred while uninstalling software module');
    } finally {
      setLoading(false);
    }
  };

  // Initial load
  useEffect(() => {
    if (deviceID) {
      fetchDeploymentUnits();
      fetchExecutionEnvironments();
    }
  }, [deviceID]);

  // Auto-refresh deployment units list every 5 seconds
  useEffect(() => {
    if (!deviceID) return;

    // Don't auto-refresh if loading or dialogs are open
    if (loading || showInstallDialog || showUninstallDialog) return;

    const interval = setInterval(() => {
      fetchDeploymentUnits();
    }, 5000); // Refresh every 5 seconds

    return () => clearInterval(interval);
  }, [deviceID, loading, showInstallDialog, showUninstallDialog]);

  // Refresh UUID when dialog opens
  useEffect(() => {
    if (showInstallDialog) {
      setInstallUuid(generateUUID());
    }
  }, [showInstallDialog]);

  return (
    <>
      <Card>
        <CardHeader 
          title="Software Module Management (LCM)"
          action={
            <Button
              variant="contained"
              startIcon={
                <SvgIcon>
                  <PlusCircleIcon />
                </SvgIcon>
              }
              onClick={() => setShowInstallDialog(true)}
              disabled={loading}
            >
              Install Module
            </Button>
          }
        />
        <Divider />
        <CardContent>
          <Stack spacing={3}>
            {error && (
              <Alert severity="error" onClose={() => setError(null)}>
                {error}
              </Alert>
            )}
            {response && (
              <Alert severity="success" onClose={() => setResponse(null)}>
                {response}
              </Alert>
            )}

            <Box>
              <Stack direction="row" spacing={2} alignItems="center" mb={2}>
                <Typography variant="h6">Installed Modules</Typography>
                <Button
                  variant="outlined"
                  size="small"
                  startIcon={
                    <SvgIcon>
                      <ArrowPathIcon />
                    </SvgIcon>
                  }
                  onClick={fetchDeploymentUnits}
                  disabled={loading}
                >
                  Refresh
                </Button>
              </Stack>

              <TableContainer component={Paper}>
                <Table>
                  <TableHead>
                    <TableRow>
                      <TableCell>Container Name</TableCell>
                      <TableCell>Version</TableCell>
                      <TableCell>Status</TableCell>
                      <TableCell>Deployment Status</TableCell>
                      <TableCell>Installed Time</TableCell>
                      <TableCell align="right">Actions</TableCell>
                    </TableRow>
                  </TableHead>
                  <TableBody>
                    {deploymentUnits.length === 0 ? (
                      <TableRow>
                        <TableCell colSpan={6} align="center">
                          {loading ? <CircularProgress size={24} /> : 'No modules installed'}
                        </TableCell>
                      </TableRow>
                    ) : (
                      deploymentUnits.map((unit) => (
                        <TableRow key={unit.instance}>
                          <TableCell>{unit.name}</TableCell>
                          <TableCell>{unit.version}</TableCell>
                          <TableCell>
                            <Chip
                              label={unit.status}
                              size="small"
                              color={
                                unit.status === 'Active' || unit.status === 'Installed'
                                  ? 'success'
                                  : unit.status === 'Failed'
                                  ? 'error'
                                  : 'default'
                              }
                            />
                          </TableCell>
                          <TableCell>
                            <Chip
                              label={unit.resolved ? 'Up to date' : 'Requires update'}
                              size="small"
                              color={unit.resolved ? 'success' : 'warning'}
                            />
                          </TableCell>
                          <TableCell>{formatTimeAgo(unit.installedTime)}</TableCell>
                          <TableCell align="right">
                            <IconButton
                              size="small"
                              color="error"
                              onClick={() => {
                                setUninstallTarget(unit);
                                setShowUninstallDialog(true);
                              }}
                              disabled={loading}
                            >
                              <SvgIcon>
                                <TrashIcon />
                              </SvgIcon>
                            </IconButton>
                          </TableCell>
                        </TableRow>
                      ))
                    )}
                  </TableBody>
                </Table>
              </TableContainer>
            </Box>
          </Stack>
        </CardContent>
      </Card>

      {/* Install Dialog */}
      <Dialog open={showInstallDialog} onClose={() => setShowInstallDialog(false)} maxWidth="md" fullWidth>
        <DialogTitle>
          <Box display="flex" justifyContent="space-between" alignItems="center">
            <Typography variant="h6">Install Software Module</Typography>
            <IconButton onClick={() => setShowInstallDialog(false)}>
              <SvgIcon>
                <XMarkIcon />
              </SvgIcon>
            </IconButton>
          </Box>
        </DialogTitle>
        <DialogContent>
          <Stack spacing={3} mt={1}>
            <Box>
              <Typography variant="subtitle2" gutterBottom>
                Docker Registry (Optional)
              </Typography>
              <Stack direction="row" spacing={2}>
                <TextField
                  label="Registry URL"
                  variant="outlined"
                  fullWidth
                  value={dockerRegistryUrl}
                  onChange={(e) => setDockerRegistryUrl(e.target.value)}
                  placeholder="http://192.168.1.24:5000"
                  disabled={loading}
                />
                <Button
                  variant="outlined"
                  onClick={fetchDockerImages}
                  disabled={loading || loadingDockerImages || !dockerRegistryUrl.trim()}
                >
                  {loadingDockerImages ? <CircularProgress size={20} /> : 'Fetch Images'}
                </Button>
              </Stack>
            </Box>

            {dockerImages.length > 0 && (
              <FormControl fullWidth>
                <InputLabel>Select from Registry</InputLabel>
                <Select
                  value=""
                  onChange={(e) => setInstallUrl(e.target.value)}
                  disabled={loading}
                  displayEmpty
                >
                  <MenuItem value="" disabled>
                    Select an image
                  </MenuItem>
                  {dockerImages.map((img, idx) => (
                    <MenuItem key={idx} value={img.fullUrl}>
                      {img.name}:{img.tag}
                    </MenuItem>
                  ))}
                </Select>
              </FormControl>
            )}
            <TextField
              label="Module URL"
              variant="outlined"
              fullWidth
              required
              value={installUrl}
              onChange={(e) => setInstallUrl(e.target.value)}
              placeholder="docker://192.168.1.24/my_cortexa53_container:v0.0.1"
              disabled={loading}
              helperText={dockerImages.length > 0 ? "Select from dropdown above or enter URL manually" : "Docker URL format: docker://host/image:tag"}
            />

            <TextField
              label="UUID"
              variant="outlined"
              fullWidth
              value={installUuid}
              onChange={(e) => setInstallUuid(e.target.value)}
              disabled={loading}
              helperText="Random UUID generated automatically"
            />

            <FormControl fullWidth>
              <InputLabel>Execution Environment</InputLabel>
              <Select
                value={installExecEnv}
                onChange={(e) => setInstallExecEnv(e.target.value)}
                disabled={loading}
              >
                {executionEnvironments.map((env) => (
                  <MenuItem key={env} value={env}>
                    {env}
                  </MenuItem>
                ))}
              </Select>
            </FormControl>

            <FormControlLabel
              control={
                <Checkbox
                  checked={installPrivileged}
                  onChange={(e) => setInstallPrivileged(e.target.checked)}
                  disabled={loading}
                />
              }
              label="Privileged (default: True)"
            />
          </Stack>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setShowInstallDialog(false)} disabled={loading}>
            Cancel
          </Button>
          <Button
            onClick={handleInstall}
            variant="contained"
            disabled={loading || !installUrl.trim()}
          >
            {loading ? <CircularProgress size={20} /> : 'Install'}
          </Button>
        </DialogActions>
      </Dialog>

      {/* Uninstall Dialog */}
      <Dialog open={showUninstallDialog} onClose={() => setShowUninstallDialog(false)}>
        <DialogTitle>Confirm Uninstall</DialogTitle>
        <DialogContent>
          <DialogContentText>
            Are you sure you want to uninstall <strong>{uninstallTarget?.name}</strong>?
            <br />
            This action cannot be undone.
          </DialogContentText>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setShowUninstallDialog(false)} disabled={loading}>
            Cancel
          </Button>
          <Button
            onClick={handleUninstall}
            variant="contained"
            color="error"
            disabled={loading}
          >
            {loading ? <CircularProgress size={20} /> : 'Uninstall'}
          </Button>
        </DialogActions>
      </Dialog>

      <Backdrop
        sx={{ color: '#fff', zIndex: (theme) => theme.zIndex.drawer + 1 }}
        open={loading}
      >
        <CircularProgress color="inherit" />
      </Backdrop>
    </>
  );
};
