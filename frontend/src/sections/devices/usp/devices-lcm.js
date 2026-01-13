import { useState, useEffect, useCallback } from 'react';
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
  Tooltip,
  Switch,
} from '@mui/material';
import { useRouter } from 'next/router';
import { useBackendContext } from 'src/contexts/backend-context';
import { keyframes } from '@mui/system';
import XMarkIcon from '@heroicons/react/24/outline/XMarkIcon';
import PlusCircleIcon from '@heroicons/react/24/outline/PlusCircleIcon';
import TrashIcon from '@heroicons/react/24/outline/TrashIcon';
import ArrowPathIcon from '@heroicons/react/24/outline/ArrowPathIcon';
import ArrowUpIcon from '@heroicons/react/24/outline/ArrowUpIcon';

// Animation keyframes for uninstalling indicator
const shimmer = keyframes`
  0% {
    background-position: -200% 0;
  }
  100% {
    background-position: 200% 0;
  }
`;

const pulse = keyframes`
  0%, 100% {
    opacity: 1;
    transform: scale(1);
  }
  50% {
    opacity: 0.7;
    transform: scale(1.05);
  }
`;

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

// Parse version tag to numeric array for sorting
// Handles various formats: v0.0.1, v.0.0.2, 0.0.3, 0.0.4.0, 00.51.11.123.4
// Also handles non-numeric tags: tag1, tag2, mytag (returns null to indicate non-numeric)
// "v0.0.1" -> [0, 0, 1]
// "v.0.0.2" -> [0, 0, 2] (handles v. prefix)
// "10.0.2" -> [10, 0, 2]
// "v1.1" -> [1, 1, 0] (assume missing parts are 0)
// "11" -> [11, 0, 0] (assume v11.0.0)
// "0.0.4.0" -> [0, 0, 4, 0] (preserves all parts)
// "00.51.11.123.4" -> [0, 51, 11, 123, 4] (handles leading zeros, preserves all parts)
// "tag1" -> null (non-numeric, use alphabetical)
// "mytag" -> null (non-numeric, use alphabetical)
function parseVersion(tag) {
  if (!tag || typeof tag !== 'string') return null;
  
  // Remove 'v' prefix if present (handles both 'v' and 'v.' cases)
  let version = tag;
  if (version.startsWith('v')) {
    version = version.substring(1);
    // If it starts with '.' after removing 'v', remove that too
    if (version.startsWith('.')) {
      version = version.substring(1);
    }
  }
  
  // Split by '.' and convert to numbers, filter out empty strings
  const parts = version.split('.')
    .filter(part => part.length > 0) // Remove empty strings from cases like "v.0.0.2"
    .map(part => {
      const num = parseInt(part, 10);
      return isNaN(num) ? null : num; // Return null for non-numeric parts
    });
  
  // If any part is non-numeric, the tag is non-numeric
  if (parts.some(part => part === null)) {
    return null;
  }
  
  // If no valid parts found, return null
  if (parts.length === 0) {
    return null;
  }
  
  // Pad missing parts with 0 (minimum 3 parts for comparison)
  while (parts.length < 3) {
    parts.push(0);
  }
  
  // Return all parts (not just first 3) to handle versions with more parts
  return parts;
}

// Sort tags in descending order (newest first)
// Numeric tags sorted numerically, non-numeric tags sorted alphabetically
// Non-numeric tags come after numeric tags
function sortTags(tags) {
  if (!tags || tags.length === 0) return [];
  
  return [...tags].sort((a, b) => {
    const aParts = parseVersion(a);
    const bParts = parseVersion(b);
    
    // If both are numeric, compare numerically
    if (aParts !== null && bParts !== null) {
      const maxLength = Math.max(aParts.length, bParts.length);
      
      for (let i = 0; i < maxLength; i++) {
        const aPart = aParts[i] || 0;
        const bPart = bParts[i] || 0;
        
        if (bPart !== aPart) {
          return bPart - aPart; // Descending order
        }
      }
      
      return 0;
    }
    
    // If one is numeric and one is not, numeric comes first (newer)
    if (aParts !== null && bParts === null) {
      return -1; // a is numeric, comes first
    }
    if (aParts === null && bParts !== null) {
      return 1; // b is numeric, comes first
    }
    
    // Both are non-numeric, sort alphabetically (descending: z->a)
    return b.localeCompare(a);
  });
}

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

// Format uptime from seconds (similar to formatTimeAgo)
const formatUptime = (seconds) => {
  if (!seconds && seconds !== 0) return 'Unknown';
  
  const totalSeconds = parseInt(seconds, 10);
  if (isNaN(totalSeconds) || totalSeconds < 0) return 'Unknown';
  
  const diffMins = Math.floor(totalSeconds / 60);
  const diffHours = Math.floor(totalSeconds / 3600);
  const diffDays = Math.floor(totalSeconds / 86400);
  
  if (diffDays > 0) {
    const hours = Math.floor((totalSeconds % 86400) / 3600);
    return `${diffDays} day${diffDays > 1 ? 's' : ''} ${hours} hour${hours !== 1 ? 's' : ''} ago`;
  } else if (diffHours > 0) {
    const mins = Math.floor((totalSeconds % 3600) / 60);
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
  const [agentRequests, setAgentRequests] = useState([]); // Device.LocalAgent.Request.*.
  const [executionUnits, setExecutionUnits] = useState(new Map()); // Map<path, {name, status}> for ExecutionUnit data
  const [executionUnitOperations, setExecutionUnitOperations] = useState(new Set()); // Set<path> for ExecutionUnits with active operations
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(null);
  const [response, setResponse] = useState(null);
  const [isSupported, setIsSupported] = useState(null); // Device.SoftwareModules. support check (null = unknown, true = supported, false = unsupported)
  const [checkingSupport, setCheckingSupport] = useState(true);
  const [updateStatuses, setUpdateStatuses] = useState(new Map()); // Map<instance, {hasUpdate: boolean, latestTag: string}>
  
  // Install dialog state
  const [showInstallDialog, setShowInstallDialog] = useState(false);
  const [installUrl, setInstallUrl] = useState('');
  const [installUuid, setInstallUuid] = useState(generateUUID());
  const [installExecEnv, setInstallExecEnv] = useState('Device.SoftwareModules.ExecEnv.1.');
  const [installPrivileged, setInstallPrivileged] = useState(true);
  
  // Update dialog state
  const [showUpdateDialog, setShowUpdateDialog] = useState(false);
  const [updateTarget, setUpdateTarget] = useState(null);
  const [updateTag, setUpdateTag] = useState('');
  const [updatePrivileged, setUpdatePrivileged] = useState(true);
  const [updateAvailableTags, setUpdateAvailableTags] = useState([]);
  const [loadingUpdateTags, setLoadingUpdateTags] = useState(false);
  // Registry URL - always use current browser hostname (for docker:// URLs)
  // The nginx proxy handles routing to the compose registry
  const getDefaultRegistryUrl = () => {
    if (typeof window !== 'undefined') {
      return window.location.hostname;
    }
    return '127.0.0.1';
  };
  const [dockerImages, setDockerImages] = useState([]); // Array of {name, tags: []}
  const [loadingDockerImages, setLoadingDockerImages] = useState(false);
  const [dockerImagesError, setDockerImagesError] = useState(null);
  const [selectedContainer, setSelectedContainer] = useState('');
  const [selectedTag, setSelectedTag] = useState('');
  const [selectedImageOption, setSelectedImageOption] = useState('custom'); // 'custom' or 'registry'
  
  // Uninstall dialog state
  const [showUninstallDialog, setShowUninstallDialog] = useState(false);
  const [uninstallTarget, setUninstallTarget] = useState(null);

  // Check if Device.SoftwareModules. is supported
  const checkSoftwareModulesSupport = async () => {
    try {
      const getSupportedDMCommand = {
        header: {
          msg_id: generateUUID(),
          msg_type: 12, // GetSupportedDM
        },
        body: {
          request: {
            get_supported_dm: {
              obj_paths: ['Device.SoftwareModules.'],
              first_level_only: false,
              return_commands: false,
              return_events: false,
              return_params: true,
            },
          },
        },
      };

      const { result, status } = await httpRequest(
        `/api/device/${deviceID}/any/generic`,
        'PUT',
        JSON.stringify(getSupportedDMCommand),
        null
      );

      if (status === 200 && result) {
        // Check if Device.SoftwareModules. is in the supported data model
        // Response structure: { req_obj_results: [{ req_obj_path: "...", supported_objs: [...] }] }
        const supported = result.req_obj_results?.some(objResult => {
          // Check if the requested path matches
          if (objResult.req_obj_path === 'Device.SoftwareModules.') {
            return true;
          }
          // Check if any supported objects start with Device.SoftwareModules.
          return objResult.supported_objs?.some(supportedObj => 
            supportedObj.supported_obj_path?.startsWith('Device.SoftwareModules.')
          );
        });
        // If supported is true, set to true; otherwise false
        const isActuallySupported = !!supported;
        console.log('SoftwareModules support check:', { supported, isActuallySupported, result });
        setIsSupported(isActuallySupported);
      } else {
        // If request fails, assume unsupported
        console.log('SoftwareModules support check failed:', { status, result });
        setIsSupported(false);
      }
    } catch (err) {
      console.error('Failed to check SoftwareModules support:', err);
      setIsSupported(false);
    } finally {
      setCheckingSupport(false);
    }
  };

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
                'Device.SoftwareModules.ExecutionUnit.*.',
                'Device.LocalAgent.Request.*.',
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

  // Parse DeploymentUnits, ExecutionUnits, and AgentRequests from GET response
  const parseDeploymentUnits = (response) => {
    const units = [];
    const requests = [];
    const execUnitsMap = new Map(); // Map<path, {name, status}>
    
    if (response.req_path_results) {
      response.req_path_results.forEach(pathResult => {
        if (pathResult.resolved_path_results) {
          pathResult.resolved_path_results.forEach(resolved => {
            const resolvedPath = resolved.resolved_path || '';
            
            // Parse Device.LocalAgent.Request.*.
            if (resolvedPath.includes('Device.LocalAgent.Request.')) {
              if (resolved.result_params) {
                const params = resolved.result_params;
                const pathParts = resolvedPath.split('.');
                const instanceIndex = pathParts[pathParts.length - 2];
                
                if (instanceIndex && !isNaN(instanceIndex)) {
                  requests.push({
                    instance: instanceIndex,
                    alias: params.Alias || '',
                    command: params.Command || '',
                    commandKey: params.CommandKey || '',
                    originator: params.Originator || '',
                    status: params.Status || 'Unknown',
                    path: resolvedPath,
                  });
                }
              }
              return; // Skip to next item
            }
            
            // Parse Device.SoftwareModules.ExecutionUnit.*.
            if (resolvedPath.includes('Device.SoftwareModules.ExecutionUnit.')) {
              if (resolved.result_params) {
                const params = resolved.result_params;
                // Normalize path: ensure it ends with a dot
                let normalizedPath = resolvedPath;
                if (!normalizedPath.endsWith('.')) {
                  normalizedPath = normalizedPath + '.';
                }
                execUnitsMap.set(normalizedPath, {
                  name: params.Name || 'Unknown',
                  status: params.Status || 'Unknown',
                  faultCode: params.ExecutionFaultCode || '',
                  faultMessage: params.ExecutionFaultMessage || '',
                  uptime: params.Uptime || '',
                  autoStart: params.AutoStart === 'true' || params.AutoStart === true,
                });
              }
              return; // Skip to next item
            }
            
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
              
              // Parse ExecutionUnitList (comma-separated list)
              const executionUnitList = params.ExecutionUnitList || '';
              let executionUnits = [];
              if (executionUnitList && executionUnitList.trim() !== '') {
                // Split by comma and normalize paths (add dot if missing)
                executionUnits = executionUnitList.split(',')
                  .map(path => path.trim())
                  .filter(path => path.length > 0)
                  .map(path => {
                    // Ensure path ends with a dot
                    if (!path.endsWith('.')) {
                      return path + '.';
                    }
                    return path;
                  });
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
                executionUnitList: executionUnits, // Store normalized paths
              });
            }
          });
        }
      });
    }
    
    // Sort deployment units by instance number (as decimal integers)
    units.sort((a, b) => {
      const instanceA = parseInt(a.instance, 10);
      const instanceB = parseInt(b.instance, 10);
      return instanceA - instanceB;
    });
    
    setDeploymentUnits(units);
    setAgentRequests(requests);
    setExecutionUnits(execUnitsMap);
  };

  // Check if a deployment unit is being uninstalled
  const isUninstalling = (unit) => {
    // Check if Status is "Uninstalling"
    if (unit.status === 'Uninstalling') {
      return true;
    }
    
    // Check if there's an active Request with Uninstall() command for this instance
    const matchingRequest = agentRequests.find(req => {
      if (req.status !== 'Active') return false;
      
      // Extract instance number from command like "Device.SoftwareModules.DeploymentUnit.3.Uninstall()"
      const commandMatch = req.command.match(/Device\.SoftwareModules\.DeploymentUnit\.(\d+)\.Uninstall\(\)/);
      if (commandMatch && commandMatch[1] === unit.instance) {
        return true;
      }
      return false;
    });
    
    return !!matchingRequest;
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
  // Uses nginx proxy to handle SSL certificate errors and CORS
  // Always uses default registry (current hostname)
  const fetchDockerImages = useCallback(async () => {
    setLoadingDockerImages(true);
    setDockerImagesError(null);
    
    try {
      // Use current browser hostname for docker:// URLs
      // The nginx proxy handles routing to the compose registry
      const registryHost = getDefaultRegistryUrl();

      // Use nginx proxy: /docker-registry/v2/_catalog
      // Nginx automatically proxies to compose registry
      // Step 1: Fetch catalog (list of repositories)
      const catalogProxyUrl = `/docker-registry/v2/_catalog`;
      const catalogResponse = await fetch(catalogProxyUrl, {
        method: 'GET',
        credentials: 'omit',
      });

      if (!catalogResponse.ok) {
        throw new Error(`Failed to fetch catalog: ${catalogResponse.status} ${catalogResponse.statusText}`);
      }

      const catalogData = await catalogResponse.json();
      
      if (!catalogData.repositories || catalogData.repositories.length === 0) {
        setDockerImages([]);
        setDockerImagesError(null);
        return;
      }

      // Step 2: Fetch tags for each repository and group by container name
      const containersMap = new Map();
      for (const repo of catalogData.repositories) {
        try {
          const tagsProxyUrl = `/docker-registry/v2/${repo}/tags/list`;
          const tagsResponse = await fetch(tagsProxyUrl, {
            method: 'GET',
            credentials: 'omit',
          });

          if (tagsResponse.ok) {
            const tagsData = await tagsResponse.json();
            if (tagsData.tags && tagsData.tags.length > 0) {
              // Sort tags in descending order (newest first)
              const sortedTags = sortTags(tagsData.tags);
              containersMap.set(repo, sortedTags);
            }
          } else {
            console.warn(`Failed to fetch tags for ${repo}: ${tagsResponse.status}`);
          }
        } catch (err) {
          console.warn(`Error fetching tags for ${repo}:`, err);
        }
      }

      // Convert map to array of {name, tags}
      const containers = Array.from(containersMap.entries()).map(([name, tags]) => ({
        name,
        tags,
      }));

      setDockerImages(containers);
      setDockerImagesError(null);
      
      // If containers found, select first container and its newest tag
      if (containers.length > 0) {
        const firstContainer = containers[0];
        setSelectedContainer(firstContainer.name);
        setSelectedTag(firstContainer.tags[0] || '');
        setSelectedImageOption('registry');
        if (firstContainer.tags[0]) {
          setInstallUrl(`docker://${registryHost}/${firstContainer.name}:${firstContainer.tags[0]}`);
        } else {
          setInstallUrl('');
        }
      } else {
        setSelectedContainer('');
        setSelectedTag('');
        setSelectedImageOption('custom');
        setInstallUrl('');
      }
    } catch (err) {
      const errorMsg = `Failed to fetch Docker images: ${err.message || err}`;
      setDockerImagesError(errorMsg);
      console.error('Docker registry fetch error:', err);
      setDockerImages([]);
      // On error, select 'custom' option
      setSelectedContainer('');
      setSelectedTag('');
      setSelectedImageOption('custom');
      setInstallUrl('');
    } finally {
      setLoadingDockerImages(false);
    }
  }, []);

  // Parse docker:// URL to extract container name and tag
  // We use the local registry from compose, so we don't need the host
  const parseDockerUrl = (url) => {
    if (!url || !url.startsWith('docker://')) {
      return null;
    }
    
    try {
      // Format: docker://host/container:tag
      const match = url.match(/^docker:\/\/[^\/]+\/([^:]+):(.+)$/);
      if (!match) {
        return null;
      }
      
      return {
        container: match[1],
        tag: match[2],
      };
    } catch (err) {
      console.error('Error parsing docker URL:', err);
      return null;
    }
  };

  // Fetch tags for a specific container from the local registry (via nginx proxy)
  // Uses the registry running in compose at 127.0.0.1:443
  const fetchContainerTags = async (containerName) => {
    try {
      // Use the local registry from compose (127.0.0.1:443)
      // Nginx proxy handles the SSL and routing
      const tagsProxyUrl = `/docker-registry/v2/${containerName}/tags/list`;
      const tagsResponse = await fetch(tagsProxyUrl, {
        method: 'GET',
        credentials: 'omit',
      });

      if (!tagsResponse.ok) {
        return null;
      }

      const tagsData = await tagsResponse.json();
      if (!tagsData.tags || tagsData.tags.length === 0) {
        return [];
      }

      // Sort tags in descending order (newest first)
      return sortTags(tagsData.tags);
    } catch (err) {
      console.error(`Error fetching tags for ${containerName}:`, err);
      return null;
    }
  };

  // Check if there's a newer version available
  // Compares current tag with available tags (already fetched)
  const checkForUpdates = (unit, availableTags) => {
    if (!unit.url || !availableTags || availableTags.length === 0) {
      return { hasUpdate: false, latestTag: null };
    }

    const urlInfo = parseDockerUrl(unit.url);
    if (!urlInfo) {
      return { hasUpdate: false, latestTag: null };
    }

    const currentTag = urlInfo.tag;
    const latestTag = availableTags[0]; // Already sorted, newest first

    // Compare versions
    const currentVersion = parseVersion(currentTag);
    const latestVersion = parseVersion(latestTag);

    // If both are numeric, compare them
    if (currentVersion !== null && latestVersion !== null) {
      const maxLength = Math.max(currentVersion.length, latestVersion.length);
      for (let i = 0; i < maxLength; i++) {
        const currentPart = currentVersion[i] || 0;
        const latestPart = latestVersion[i] || 0;
        if (latestPart > currentPart) {
          return { hasUpdate: true, latestTag };
        }
        if (latestPart < currentPart) {
          return { hasUpdate: false, latestTag };
        }
      }
      // Versions are equal
      return { hasUpdate: false, latestTag };
    }

    // If non-numeric, compare alphabetically
    if (currentVersion === null && latestVersion === null) {
      return { hasUpdate: latestTag > currentTag, latestTag };
    }

    // Mixed: numeric tags are considered newer than non-numeric
    if (currentVersion === null && latestVersion !== null) {
      return { hasUpdate: true, latestTag };
    }
    if (currentVersion !== null && latestVersion === null) {
      return { hasUpdate: false, latestTag };
    }

    return { hasUpdate: false, latestTag };
  };

  // Check update status for all deployment units
  // Optimized: fetches tags once per unique container
  useEffect(() => {
    const checkAllUpdates = async () => {
      if (deploymentUnits.length === 0) {
        setUpdateStatuses(new Map());
        return;
      }

      // Step 1: Collect unique container names from all deployment units
      const containerMap = new Map(); // Map<containerName, Set<unit>>
      deploymentUnits.forEach((unit) => {
        if (!unit.url) return;
        
        const urlInfo = parseDockerUrl(unit.url);
        if (!urlInfo) return;

        if (!containerMap.has(urlInfo.container)) {
          containerMap.set(urlInfo.container, new Set());
        }
        containerMap.get(urlInfo.container).add(unit);
      });

      // Step 2: Fetch tags for each unique container once
      const tagsCache = new Map(); // Map<containerName, tags[]>
      const fetchPromises = Array.from(containerMap.keys()).map(async (containerName) => {
        const tags = await fetchContainerTags(containerName);
        tagsCache.set(containerName, tags || []);
      });

      await Promise.all(fetchPromises);

      // Step 3: Check each deployment unit against cached tags
      const statusMap = new Map();
      deploymentUnits.forEach((unit) => {
        if (!unit.url) {
          statusMap.set(unit.instance, { hasUpdate: false, latestTag: null });
          return;
        }

        const urlInfo = parseDockerUrl(unit.url);
        if (!urlInfo) {
          statusMap.set(unit.instance, { hasUpdate: false, latestTag: null });
          return;
        }

        const availableTags = tagsCache.get(urlInfo.container) || [];
        const updateInfo = checkForUpdates(unit, availableTags);
        statusMap.set(unit.instance, updateInfo);
      });

      setUpdateStatuses(statusMap);
    };

    checkAllUpdates();
  }, [deploymentUnits]);

  // Auto-fetch images when install dialog opens
  useEffect(() => {
    if (showInstallDialog) {
      fetchDockerImages();
    } else if (!showInstallDialog) {
      // Reset state when dialog closes
      setDockerImages([]);
      setDockerImagesError(null);
      setSelectedContainer('');
      setSelectedTag('');
      setSelectedImageOption('custom');
      setInstallUrl('');
    }
  }, [showInstallDialog, fetchDockerImages]);

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
          setSelectedContainer('');
          setSelectedTag('');
          setSelectedImageOption('custom');
          setInstallUrl('');
          setInstallUrl('');
          setInstallUuid(generateUUID());
          // Refresh the list after 1 second
          setTimeout(() => {
            fetchDeploymentUnits();
          }, 2000);
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
          setSelectedContainer('');
          setSelectedTag('');
          setSelectedImageOption('custom');
          setInstallUrl('');
          setInstallUrl('');
          setInstallUuid(generateUUID());
          // Refresh the list after 1 second
          setTimeout(() => {
            fetchDeploymentUnits();
          }, 2000);
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

  // Open update dialog
  const handleUpdate = async (unit) => {
    const urlInfo = parseDockerUrl(unit.url);
    if (!urlInfo) {
      setError('Invalid container URL');
      return;
    }

    setUpdateTarget(unit);
    setUpdatePrivileged(true);
    setUpdateTag('');
    setUpdateAvailableTags([]);
    setShowUpdateDialog(true);
  };

  // Fetch tags for update dialog
  useEffect(() => {
    const fetchUpdateTags = async () => {
      if (!showUpdateDialog || !updateTarget) return;

      const urlInfo = parseDockerUrl(updateTarget.url);
      if (!urlInfo) return;

      setLoadingUpdateTags(true);
      try {
        const tags = await fetchContainerTags(urlInfo.container);
        if (tags && tags.length > 0) {
          setUpdateAvailableTags(tags);
          // Set default to latest tag (first in sorted list)
          if (!updateTag && tags.length > 0) {
            setUpdateTag(tags[0]);
          }
        } else {
          setUpdateAvailableTags([]);
        }
      } catch (err) {
        console.error('Error fetching update tags:', err);
        setUpdateAvailableTags([]);
      } finally {
        setLoadingUpdateTags(false);
      }
    };

    fetchUpdateTags();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [showUpdateDialog, updateTarget]);

  // Handle update submit
  const handleUpdateSubmit = async () => {
    if (!updateTarget || !updateTag) {
      setError('Please select a tag');
      return;
    }

    const urlInfo = parseDockerUrl(updateTarget.url);
    if (!urlInfo) {
      setError('Invalid container URL');
      return;
    }

    const registryHost = getDefaultRegistryUrl();
    const updateUrl = `docker://${registryHost}/${urlInfo.container}:${updateTag}`;

    setLoading(true);
    setError(null);
    setResponse(null);

    try {
      const instanceNum = updateTarget.instance;
      const updateCommandPath = `Device.SoftwareModules.DeploymentUnit.${instanceNum}.Update()`;

      // Step 1: Ensure subscription exists
      const subscriptionReady = await ensureSubscription(updateCommandPath);
      if (!subscriptionReady) {
        return;
      }

      // Step 2: Send Update command
      const updateCommand = {
        header: {
          msg_id: generateUUID(),
          msg_type: 6, // OPERATE
        },
        body: {
          request: {
            operate: {
              command: updateCommandPath,
              command_key: 'Update',
              send_resp: true,
              input_args: {
                URL: updateUrl,
                Privileged: updatePrivileged.toString(),
              },
            },
          },
        },
      };

      const { result, status } = await httpRequest(
        `/api/device/${deviceID}/any/generic`,
        'PUT',
        JSON.stringify(updateCommand),
        null
      );

      if (status === 200) {
        const operationResult = result?.operation_results?.[0];
        const operationResp = operationResult?.OperationResp;
        
        // Check for OperSuccess
        if (operationResp?.OperSuccess !== undefined) {
          setResponse(`Container updated to ${updateTag} successfully`);
          setError(null);
          setShowUpdateDialog(false);
          setUpdateTarget(null);
          setUpdateTag('');
          // Refresh the list after 1 second
          setTimeout(() => {
            fetchDeploymentUnits();
          }, 2000);
        } 
        // Check for CmdFailure
        else if (operationResp?.CmdFailure) {
          const errorMsg = operationResp.CmdFailure.err_msg || 
                          `Update failed: ${operationResp.CmdFailure.err_code || 'Unknown error'}`;
          setError(errorMsg);
          setResponse(null);
        } 
        // Fallback: check if there's any indication of success
        else if (!operationResp?.CmdFailure) {
          setResponse(`Container update to ${updateTag} initiated successfully`);
          setError(null);
          setShowUpdateDialog(false);
          setUpdateTarget(null);
          setUpdateTag('');
          // Refresh the list after 1 second
          setTimeout(() => {
            fetchDeploymentUnits();
          }, 2000);
        } else {
          setError('Unknown response format from update operation');
          setResponse(null);
        }
      } else {
        setError(result?.message || result || 'Failed to update container');
        setResponse(null);
      }
    } catch (err) {
      setError(err.message || 'An error occurred while updating container');
      setResponse(null);
    } finally {
      setLoading(false);
    }
  };

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

  // Handle Start/Stop for ExecutionUnit
  const handleExecutionUnitStartStop = async (execUnitPath, currentStatus) => {
    // Determine requested state based on current status
    // If Active or Starting -> Stop (Idle)
    // If Idle -> Start (Active)
    // If Stopping -> reject (shouldn't happen, but handle gracefully)
    let requestedState;
    if (currentStatus === 'Active' || currentStatus === 'Starting') {
      requestedState = 'Idle'; // Stop
    } else if (currentStatus === 'Idle') {
      requestedState = 'Active'; // Start
    } else if (currentStatus === 'Stopping') {
      setError('Cannot change state while ExecutionUnit is stopping');
      return;
    } else {
      setError(`Cannot change state from ${currentStatus}`);
      return;
    }

    // Block this ExecutionUnit
    setExecutionUnitOperations(prev => new Set(prev).add(execUnitPath));
    setLoading(true);
    setError(null);
    setResponse(null);

    try {
      // Remove trailing dot from path if present for command
      const commandPath = execUnitPath.endsWith('.') 
        ? execUnitPath.slice(0, -1) + '.SetRequestedState()'
        : execUnitPath + '.SetRequestedState()';
      
      const operateCommand = {
        header: {
          msg_id: generateUUID(),
          msg_type: 6, // OPERATE
        },
        body: {
          request: {
            operate: {
              command: commandPath,
              command_key: 'SetRequestedState',
              send_resp: true,
              input_args: {
                RequestedState: requestedState,
              },
            },
          },
        },
      };

      const { result, status } = await httpRequest(
        `/api/device/${deviceID}/any/generic`,
        'PUT',
        JSON.stringify(operateCommand),
        null
      );

      if (status === 200) {
        const operationResult = result?.operation_results?.[0];
        const operationResp = operationResult?.OperationResp;
        
        // Check for ReqOutputArgs (success response for SetRequestedState)
        if (operationResp?.ReqOutputArgs !== undefined) {
          // Ignore _retval, just treat as success
          setResponse(`ExecutionUnit ${requestedState === 'Active' ? 'started' : 'stopped'} successfully`);
          setError(null);
          // Refresh the list after a short delay
          setTimeout(() => {
            fetchDeploymentUnits();
          }, 1000);
        } 
        // Check for CmdFailure
        else if (operationResp?.CmdFailure) {
          const errorMsg = operationResp.CmdFailure.err_msg || 
                          `Failed to ${requestedState === 'Active' ? 'start' : 'stop'} ExecutionUnit`;
          setError(errorMsg);
          setResponse(null);
        } 
        // Fallback
        else {
          setError('Unknown response format from SetRequestedState operation');
          setResponse(null);
        }
      } else {
        setError(result?.message || result || `Failed to ${requestedState === 'Active' ? 'start' : 'stop'} ExecutionUnit`);
        setResponse(null);
      }
    } catch (err) {
      setError(err.message || `An error occurred while ${requestedState === 'Active' ? 'starting' : 'stopping'} ExecutionUnit`);
      setResponse(null);
    } finally {
      setLoading(false);
      // Unblock this ExecutionUnit
      setExecutionUnitOperations(prev => {
        const next = new Set(prev);
        next.delete(execUnitPath);
        return next;
      });
    }
  };

  // Handle AutoStart toggle for ExecutionUnit
  const handleAutoStartToggle = async (execUnitPath, currentValue) => {
    const newValue = !currentValue;
    
    // Block this ExecutionUnit
    setExecutionUnitOperations(prev => new Set(prev).add(execUnitPath));
    setLoading(true);
    setError(null);
    setResponse(null);

    try {
      const setCommand = {
        header: {
          msg_id: generateUUID(),
          msg_type: 4, // SET
        },
        body: {
          request: {
            set: {
              allow_partial: false,
              update_objs: [
                {
                  obj_path: execUnitPath,
                  param_settings: [
                    {
                      param: 'AutoStart',
                      value: newValue.toString(),
                      required: true,
                    },
                  ],
                },
              ],
            },
          },
        },
      };

      const { result, status } = await httpRequest(
        `/api/device/${deviceID}/any/generic`,
        'PUT',
        JSON.stringify(setCommand),
        null
      );

      if (status === 200) {
        // Check for success in the response
        const updateResult = result?.updated_obj_results?.[0];
        if (updateResult?.oper_status?.OperStatus?.OperSuccess !== undefined) {
          setResponse(`AutoStart ${newValue ? 'enabled' : 'disabled'} successfully`);
          setError(null);
          // Refresh the list after a short delay
          setTimeout(() => {
            fetchDeploymentUnits();
          }, 1000);
        } else {
          const errorMsg = updateResult?.oper_status?.OperStatus?.OperFailure?.err_msg || 
                          'Failed to update AutoStart';
          setError(errorMsg);
          setResponse(null);
        }
      } else {
        setError(result?.message || result || 'Failed to update AutoStart');
        setResponse(null);
      }
    } catch (err) {
      setError(err.message || 'An error occurred while updating AutoStart');
      setResponse(null);
    } finally {
      setLoading(false);
      // Unblock this ExecutionUnit
      setExecutionUnitOperations(prev => {
        const next = new Set(prev);
        next.delete(execUnitPath);
        return next;
      });
    }
  };

  // Initial load - check support first
  useEffect(() => {
    if (deviceID) {
      checkSoftwareModulesSupport();
    }
  }, [deviceID]);

  // Fetch data after support check completes
  useEffect(() => {
    if (deviceID && isSupported === true && !checkingSupport) {
      fetchDeploymentUnits();
      fetchExecutionEnvironments();
    }
  }, [deviceID, isSupported, checkingSupport]);

  // Auto-refresh deployment units list every 10 seconds
  useEffect(() => {
    if (!deviceID || isSupported !== true) return;

    // Don't auto-refresh if loading or dialogs are open
    if (loading || showInstallDialog || showUninstallDialog || showUpdateDialog) return;

    const interval = setInterval(() => {
      fetchDeploymentUnits();
    }, 10000); // Refresh every 10 seconds

    return () => clearInterval(interval);
  }, [deviceID, loading, showInstallDialog, showUninstallDialog, showUpdateDialog, isSupported]);

  // Refresh UUID when dialog opens
  useEffect(() => {
    if (showInstallDialog) {
      setInstallUuid(generateUUID());
    }
  }, [showInstallDialog]);

  return (
    <>
      <Card sx={{ position: 'relative' }}>
        {isSupported === false && !checkingSupport && (
          <Box
            sx={{
              position: 'absolute',
              top: 0,
              left: 0,
              right: 0,
              bottom: 0,
              backgroundColor: 'rgba(0, 0, 0, 0.7)',
              zIndex: 10,
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
            }}
          >
            <Alert 
              severity="error"
              sx={{
                backgroundColor: 'rgba(211, 47, 47, 0.95)',
                color: 'white',
                fontSize: '1.1rem',
                fontWeight: 500,
                minWidth: 400,
                '& .MuiAlert-icon': {
                  color: 'white',
                },
              }}
            >
              Device.SoftwareModules. nodes are unsupported
            </Alert>
          </Box>
        )}
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
              disabled={loading || isSupported === false || checkingSupport}
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
                  disabled={loading || isSupported === false || checkingSupport}
                >
                  Refresh
                </Button>
              </Stack>

              <TableContainer component={Paper} sx={{ position: 'relative' }}>
                {loading && (
                  <Box
                    sx={{
                      position: 'absolute',
                      top: 0,
                      left: 0,
                      right: 0,
                      bottom: 0,
                      backgroundColor: 'rgba(127, 127, 127, 0.01)',
                      display: 'flex',
                      alignItems: 'center',
                      justifyContent: 'center',
                      zIndex: 10,
                    }}
                  >
                    <CircularProgress sx={{ color: '#ff9800' }} />
                  </Box>
                )}
                <Table>
                  <TableHead>
                    <TableRow>
                      <TableCell>Container Name</TableCell>
                      <TableCell>Version</TableCell>
                      <TableCell>Status</TableCell>
                      <TableCell>Update Status</TableCell>
                      <TableCell>Install Time/Uptime</TableCell>
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
                      deploymentUnits.flatMap((unit) => {
                        const uninstalling = isUninstalling(unit);
                        const rows = [
                          // Main DeploymentUnit row
                          <TableRow 
                            key={unit.instance}
                            sx={{
                              position: 'relative',
                              opacity: uninstalling ? 0.5 : 1,
                              transition: 'opacity 0.3s ease-in-out',
                              backgroundColor: uninstalling ? 'rgba(255, 152, 0, 0.1)' : 'transparent',
                              backgroundImage: uninstalling ? 'linear-gradient(90deg, transparent, rgba(255, 152, 0, 0.2), transparent)' : 'none',
                              backgroundSize: uninstalling ? '200% 100%' : 'auto',
                              animation: uninstalling ? `${shimmer} 2s infinite linear` : 'none',
                            }}
                          >
                            <TableCell>{unit.name}</TableCell>
                            <TableCell>{unit.version}</TableCell>
                            <TableCell>
                              <Chip
                                label={uninstalling ? 'Uninstalling...' : unit.status}
                                size="small"
                                color={
                                  uninstalling
                                    ? 'warning'
                                    : unit.status === 'Active' || unit.status === 'Installed'
                                    ? 'success'
                                    : unit.status === 'Failed'
                                    ? 'error'
                                    : 'default'
                                }
                                sx={uninstalling ? {
                                  animation: `${pulse} 1.5s ease-in-out infinite`,
                                } : {}}
                              />
                            </TableCell>
                            <TableCell>
                              {(() => {
                                const updateInfo = updateStatuses.get(unit.instance);
                                if (!updateInfo) {
                                  return (
                                    <Chip
                                      label="Checking..."
                                      size="small"
                                      color="default"
                                    />
                                  );
                                }
                                if (updateInfo.hasUpdate) {
                                  return (
                                    <Chip
                                      label={`Update available (${updateInfo.latestTag})`}
                                      size="small"
                                      color="warning"
                                    />
                                  );
                                }
                                return (
                                  <Chip
                                    label="Up to date"
                                    size="small"
                                    color="success"
                                  />
                                );
                              })()}
                            </TableCell>
                            <TableCell>
                              <Tooltip title="Installed Time" arrow>
                                <span>{formatTimeAgo(unit.installedTime)}</span>
                              </Tooltip>
                            </TableCell>
                            <TableCell align="right">
                              <Box sx={{ display: 'flex', gap: 1, justifyContent: 'flex-end' }}>
                                <IconButton
                                  size="small"
                                  color="error"
                                  onClick={() => handleUpdate(unit)}
                                  disabled={loading || uninstalling || isSupported === false || checkingSupport || !unit.url}
                                  sx={{
                                    color: 'error.main',
                                    '&:hover': {
                                      backgroundColor: 'error.light',
                                      color: 'error.dark',
                                    },
                                  }}
                                  title="Update container (upgrade/downgrade)"
                                >
                                  <SvgIcon>
                                    <ArrowUpIcon />
                                  </SvgIcon>
                                </IconButton>
                                <IconButton
                                  size="small"
                                  color="error"
                                  onClick={() => {
                                    setUninstallTarget(unit);
                                    setShowUninstallDialog(true);
                                  }}
                                  disabled={loading || uninstalling || isSupported === false || checkingSupport}
                                  title="Uninstall container"
                                >
                                  <SvgIcon>
                                    <TrashIcon />
                                  </SvgIcon>
                                </IconButton>
                              </Box>
                            </TableCell>
                          </TableRow>
                        ];
                        
                        // Add ExecutionUnit subrows if they exist
                        if (unit.executionUnitList && unit.executionUnitList.length > 0) {
                          unit.executionUnitList.forEach((execUnitPath) => {
                            const execUnit = executionUnits.get(execUnitPath);
                            // Only add subrow if ExecutionUnit data exists in the response
                            if (execUnit) {
                              const isOperating = executionUnitOperations.has(execUnitPath);
                              rows.push(
                                <TableRow 
                                  key={`${unit.instance}-${execUnitPath}`}
                                  sx={{
                                    position: 'relative',
                                    opacity: isOperating ? 0.5 : 1,
                                    transition: 'opacity 0.3s ease-in-out',
                                    backgroundColor: isOperating ? 'rgba(255, 152, 0, 0.1)' : 'rgba(0, 0, 0, 0.02)',
                                    backgroundImage: isOperating ? 'linear-gradient(90deg, transparent, rgba(255, 152, 0, 0.2), transparent)' : 'none',
                                    backgroundSize: isOperating ? '200% 100%' : 'auto',
                                    animation: isOperating ? `${shimmer} 2s infinite linear` : 'none',
                                  }}
                                >
                                  <TableCell sx={{ pl: 4 }}>
                                    <Typography variant="body2" color="text.secondary">
                                      └ {execUnit.name}
                                    </Typography>
                                  </TableCell>
                                  <TableCell></TableCell>
                                  <TableCell>
                                    <Chip
                                      label={execUnit.status}
                                      size="small"
                                      color={
                                        execUnit.status === 'Active' || execUnit.status === 'Running'
                                          ? 'success'
                                          : execUnit.status === 'Failed' || execUnit.status === 'Stopped'
                                          ? 'error'
                                          : 'default'
                                      }
                                    />
                                  </TableCell>
                                  <TableCell>
                                    {execUnit.faultCode ? (
                                      <Tooltip 
                                        title={execUnit.faultMessage || 'No fault message available'} 
                                        arrow
                                      >
                                        <Chip
                                          label={execUnit.faultCode}
                                          size="small"
                                          color={execUnit.faultCode === 'NoFault' ? 'success' : 'error'}
                                        />
                                      </Tooltip>
                                    ) : (
                                      <Typography variant="body2" color="text.disabled">
                                        -
                                      </Typography>
                                    )}
                                  </TableCell>
                                  <TableCell>
                                    {execUnit.uptime !== '' ? (
                                      <Tooltip title="Uptime" arrow>
                                        <span>{formatUptime(execUnit.uptime)}</span>
                                      </Tooltip>
                                    ) : (
                                      <Typography variant="body2" color="text.disabled">
                                        -
                                      </Typography>
                                    )}
                                  </TableCell>
                                  <TableCell align="right">
                                    <Box sx={{ display: 'flex', gap: 1, justifyContent: 'flex-end', alignItems: 'center' }}>
                                      {(() => {
                                        const isOperating = executionUnitOperations.has(execUnitPath);
                                        return (
                                          <>
                                            <Tooltip title="AutoStart" arrow>
                                              <FormControlLabel
                                                control={
                                                  <Switch
                                                    checked={execUnit.autoStart || false}
                                                    onChange={() => handleAutoStartToggle(execUnitPath, execUnit.autoStart)}
                                                    size="small"
                                                    disabled={loading || isSupported === false || checkingSupport || isOperating}
                                                  />
                                                }
                                                label=""
                                                sx={{ m: 0 }}
                                              />
                                            </Tooltip>
                                            {(() => {
                                              const isActive = execUnit.status === 'Active' || execUnit.status === 'Starting';
                                              const isIdle = execUnit.status === 'Idle';
                                              const isStopping = execUnit.status === 'Stopping';
                                              const canToggle = isActive || isIdle;
                                              
                                              return (
                                                <Tooltip 
                                                  title={
                                                    isOperating
                                                      ? 'Operation in progress...'
                                                      : isStopping 
                                                      ? 'Cannot change state while stopping' 
                                                      : canToggle 
                                                      ? (isActive ? 'Stop ExecutionUnit' : 'Start ExecutionUnit')
                                                      : `Cannot change state from ${execUnit.status}`
                                                  } 
                                                  arrow
                                                >
                                                  <span>
                                                    <Button
                                                      variant={isActive ? 'contained' : 'outlined'}
                                                      color={isActive ? 'error' : 'success'}
                                                      size="small"
                                                      onClick={() => handleExecutionUnitStartStop(execUnitPath, execUnit.status)}
                                                      disabled={loading || isSupported === false || checkingSupport || !canToggle || isOperating}
                                                    >
                                                      {isOperating ? '...' : (isActive ? 'Stop' : isIdle ? 'Start' : execUnit.status)}
                                                    </Button>
                                                  </span>
                                                </Tooltip>
                                              );
                                            })()}
                                          </>
                                        );
                                      })()}
                                    </Box>
                                  </TableCell>
                                </TableRow>
                              );
                            }
                          });
                        }
                        
                        return rows;
                      })
                    )
                    }
                  </TableBody>
                </Table>
              </TableContainer>
            </Box>
          </Stack>
        </CardContent>
      </Card>

      {/* Install Dialog */}
      <Dialog 
        open={showInstallDialog} 
        onClose={() => {
          setShowInstallDialog(false);
          setSelectedContainer('');
          setSelectedTag('');
          setSelectedImageOption('custom');
          setInstallUrl('');
          setSelectedContainer('');
          setSelectedTag('');
          setSelectedImageOption('custom');
          setInstallUrl('');
        }} 
        maxWidth="md" 
        fullWidth
      >
        <DialogTitle>
          <Box display="flex" justifyContent="space-between" alignItems="center">
            <Typography variant="h6">Install Software Module</Typography>
            <IconButton onClick={() => {
              setShowInstallDialog(false);
          setSelectedContainer('');
          setSelectedTag('');
          setSelectedImageOption('custom');
          setInstallUrl('');
              setSelectedContainer('');
              setSelectedTag('');
              setSelectedImageOption('custom');
              setInstallUrl('');
            }}>
              <SvgIcon>
                <XMarkIcon />
              </SvgIcon>
            </IconButton>
          </Box>
        </DialogTitle>
        <DialogContent>
          <Stack spacing={3} mt={1}>
            {/* Error message for Docker registry fetch */}
            {dockerImagesError && (
              <Alert severity="error" onClose={() => setDockerImagesError(null)}>
                {dockerImagesError}
              </Alert>
            )}

            {loadingDockerImages && (
              <Box display="flex" justifyContent="center" alignItems="center" py={2}>
                <CircularProgress size={24} sx={{ mr: 2 }} />
                <Typography variant="body2" color="text.secondary">
                  Loading containers from registry...
                </Typography>
              </Box>
            )}

            {/* Container selection dropdown */}
            <FormControl fullWidth>
              <InputLabel id="container-select-label">Container</InputLabel>
              <Select
                labelId="container-select-label"
                value={
                  loadingDockerImages 
                    ? 'loading' 
                    : (selectedImageOption === 'custom' ? 'custom' : selectedContainer)
                }
                onChange={(e) => {
                  const value = e.target.value;
                  if (value === 'loading') return;
                  if (value === 'custom') {
                    setSelectedContainer('');
                    setSelectedTag('');
                    setSelectedImageOption('custom');
                    // Don't clear installUrl - preserve user input if they switch back to custom
                  } else {
                    const container = dockerImages.find(c => c.name === value);
                    if (container && container.tags.length > 0) {
                      setSelectedContainer(value);
                      const newestTag = container.tags[0];
                      setSelectedTag(newestTag);
                      setSelectedImageOption('registry');
                      const registryHost = getDefaultRegistryUrl().replace(/^https?:\/\//, '').replace(/\/$/, '');
                      setInstallUrl(`docker://${registryHost}/${value}:${newestTag}`);
                    } else {
                      setSelectedContainer(value);
                      setSelectedTag('');
                      setSelectedImageOption('registry');
                      setInstallUrl('');
                    }
                  }
                }}
                disabled={loading || loadingDockerImages}
              >
                {loadingDockerImages ? (
                  <MenuItem value="loading" disabled>
                    <CircularProgress size={16} sx={{ mr: 1 }} />
                    Loading containers...
                  </MenuItem>
                ) : dockerImages.length > 0 ? (
                  [
                    ...dockerImages.map((container) => (
                      <MenuItem key={container.name} value={container.name}>
                        {container.name}
                      </MenuItem>
                    )),
                    <MenuItem key="custom" value="custom">Custom URL</MenuItem>
                  ]
                ) : (
                  <MenuItem value="custom">Custom URL</MenuItem>
                )}
              </Select>
            </FormControl>

            {/* Tag selection dropdown - only shown when a container is selected */}
            {selectedContainer && selectedImageOption === 'registry' && (
              <FormControl fullWidth>
                <InputLabel id="tag-select-label">Tag</InputLabel>
                <Select
                  labelId="tag-select-label"
                  value={selectedTag}
                  onChange={(e) => {
                    const tag = e.target.value;
                    setSelectedTag(tag);
                    const registryHost = getDefaultRegistryUrl().replace(/^https?:\/\//, '').replace(/\/$/, '');
                    setInstallUrl(`docker://${registryHost}/${selectedContainer}:${tag}`);
                  }}
                  disabled={loading || loadingDockerImages}
                >
                  {dockerImages
                    .find(c => c.name === selectedContainer)
                    ?.tags.map((tag) => (
                      <MenuItem key={tag} value={tag}>
                        {tag}
                      </MenuItem>
                    ))}
                </Select>
              </FormControl>
            )}

            {/* Custom URL input - only shown when "Custom URL" is selected */}
            {selectedImageOption === 'custom' && (
              <TextField
                label="Module URL"
                variant="outlined"
                fullWidth
                required
                value={installUrl}
                onChange={(e) => setInstallUrl(e.target.value)}
                placeholder="docker://<host>/<container_name>:<tag>"
                disabled={loading}
                helperText="Docker URL format: docker://host/image:tag"
              />
            )}

            <Stack direction="row" spacing={2} alignItems="flex-start">
              <TextField
                label="UUID"
                variant="outlined"
                fullWidth
                value={installUuid}
                onChange={(e) => setInstallUuid(e.target.value)}
                disabled={loading}
                helperText="Random UUID generated automatically"
              />
              <Button
                variant="outlined"
                onClick={() => setInstallUuid(generateUUID())}
                disabled={loading}
                sx={{ mt: 1, minWidth: 150 }}
              >
                Generate
              </Button>
            </Stack>

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
          <Button onClick={() => {
            setShowInstallDialog(false);
          setSelectedContainer('');
          setSelectedTag('');
          setSelectedImageOption('custom');
          setInstallUrl('');
            setSelectedContainer('');
            setSelectedTag('');
            setSelectedImageOption('custom');
            setInstallUrl('');
          }} disabled={loading}>
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

      {/* Update Dialog */}
      <Dialog 
        open={showUpdateDialog} 
        onClose={() => {
          setShowUpdateDialog(false);
          setUpdateTarget(null);
          setUpdateTag('');
          setUpdateAvailableTags([]);
        }} 
        maxWidth="md" 
        fullWidth
      >
        <DialogTitle>
          <Box display="flex" justifyContent="space-between" alignItems="center">
            <Typography variant="h6">Update Container</Typography>
            <IconButton onClick={() => {
              setShowUpdateDialog(false);
              setUpdateTarget(null);
              setUpdateTag('');
              setUpdateAvailableTags([]);
            }}>
              <SvgIcon>
                <XMarkIcon />
              </SvgIcon>
            </IconButton>
          </Box>
        </DialogTitle>
        <DialogContent>
          <Stack spacing={3} mt={1}>
            {updateTarget && (() => {
              const urlInfo = parseDockerUrl(updateTarget.url);
              const currentTag = urlInfo?.tag || '';
              
              return (
                <>
                  {/* Container name (read-only) */}
                  <TextField
                    label="Container Name"
                    variant="outlined"
                    fullWidth
                    value={urlInfo?.container || ''}
                    disabled
                    helperText="Container name cannot be changed"
                  />

                  {/* Available tags dropdown */}
                  <FormControl fullWidth>
                    <InputLabel id="update-tag-select-label">Available Tags</InputLabel>
                    <Select
                      labelId="update-tag-select-label"
                      value={updateTag}
                      onChange={(e) => setUpdateTag(e.target.value)}
                      disabled={loading || loadingUpdateTags || updateAvailableTags.length === 0}
                      label="Available Tags"
                    >
                      {loadingUpdateTags ? (
                        <MenuItem value="" disabled>
                          <CircularProgress size={16} sx={{ mr: 1 }} />
                          Loading tags...
                        </MenuItem>
                      ) : updateAvailableTags.length > 0 ? (
                        updateAvailableTags.map((tag) => {
                          const isCurrentTag = tag === currentTag;
                          return (
                            <MenuItem 
                              key={tag} 
                              value={tag}
                              disabled={isCurrentTag}
                              sx={isCurrentTag ? {
                                color: 'text.disabled',
                                fontStyle: 'italic',
                              } : {}}
                            >
                              {tag} {isCurrentTag ? '(current)' : ''}
                            </MenuItem>
                          );
                        })
                      ) : (
                        <MenuItem value="" disabled>
                          No tags available
                        </MenuItem>
                      )}
                    </Select>
                  </FormControl>

                  {/* Privileged checkbox */}
                  <FormControlLabel
                    control={
                      <Checkbox
                        checked={updatePrivileged}
                        onChange={(e) => setUpdatePrivileged(e.target.checked)}
                        disabled={loading}
                      />
                    }
                    label="Privileged (default: True)"
                  />
                </>
              );
            })()}
          </Stack>
        </DialogContent>
        <DialogActions>
          <Button 
            onClick={() => {
              setShowUpdateDialog(false);
              setUpdateTarget(null);
              setUpdateTag('');
              setUpdateAvailableTags([]);
            }} 
            disabled={loading}
          >
            Cancel
          </Button>
          <Button
            onClick={handleUpdateSubmit}
            variant="contained"
            disabled={loading || !updateTag || loadingUpdateTags}
          >
            {loading ? <CircularProgress size={20} /> : 'Update'}
          </Button>
        </DialogActions>
      </Dialog>
    </>
  );
};
