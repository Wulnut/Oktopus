import { useState, useEffect, useCallback } from 'react';
import Head from 'next/head';
import {
  Box,
  Button,
  Container,
  Stack,
  SvgIcon,
  Typography,
  Card,
  CardContent,
  CardHeader,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Paper,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  TextField,
  Alert,
  CircularProgress,
  IconButton,
  Chip,
  Tooltip,
  Select,
  MenuItem,
  Menu,
  FormControl,
  InputLabel,
} from '@mui/material';
import PlusIcon from '@heroicons/react/24/solid/PlusIcon';
import XMarkIcon from '@heroicons/react/24/outline/XMarkIcon';
import TrashIcon from '@heroicons/react/24/outline/TrashIcon';
import EllipsisVerticalIcon from '@heroicons/react/24/outline/EllipsisVerticalIcon';
import ArrowUpTrayIcon from '@heroicons/react/24/outline/ArrowUpTrayIcon';
import { Layout as DashboardLayout } from 'src/layouts/dashboard/layout';
import { useBackendContext } from 'src/contexts/backend-context';
import { useTenant } from 'src/contexts/tenant-context';
import { useRouter } from 'next/router';

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

const Page = () => {
  const router = useRouter();
  const { httpRequest } = useBackendContext();
  const { tenantSlug } = useTenant();
  const [containers, setContainers] = useState([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [success, setSuccess] = useState(null);
  
  // Upload dialog state
  const [showUploadDialog, setShowUploadDialog] = useState(false);
  const [uploading, setUploading] = useState(false);
  const [selectedContainerOption, setSelectedContainerOption] = useState(''); // '' or 'new' or container name
  const [customContainerName, setCustomContainerName] = useState(''); // For new containers
  const [containerTag, setContainerTag] = useState('');
  const [containerFile, setContainerFile] = useState(null);
  const [uploadError, setUploadError] = useState(null);
  const [tagError, setTagError] = useState(null);
  
  // Delete state
  const [deleting, setDeleting] = useState({});
  const [deleteConfirm, setDeleteConfirm] = useState(null);
  const [deleteContainerConfirm, setDeleteContainerConfirm] = useState(null);
  
  // Selected tags state: { [containerName]: selectedTag }
  const [selectedTags, setSelectedTags] = useState({});
  
  // Menu anchor for container actions
  const [menuAnchor, setMenuAnchor] = useState(null);
  const [menuContainer, setMenuContainer] = useState(null);

  // Fetch containers list
  const fetchContainers = useCallback(async () => {
    setLoading(true);
    setError(null);
    
    try {
      const authHeaders = { 'Authorization': localStorage.getItem('token') };
      const tenantPrefix = tenantSlug + '/';

      // Use nginx proxy to fetch from compose registry
      const catalogResponse = await fetch('/docker-registry/v2/_catalog', {
        method: 'GET',
        headers: authHeaders,
      });

      if (!catalogResponse.ok) {
        throw new Error(`Failed to fetch catalog: ${catalogResponse.status}`);
      }

      const catalogData = await catalogResponse.json();

      // Filter repos belonging to this tenant
      const tenantRepos = (catalogData.repositories || []).filter(
        repo => repo.startsWith(tenantPrefix)
      );

      if (tenantRepos.length === 0) {
        setContainers([]);
        setLoading(false);
        return;
      }

      // Fetch tags for each repository
      const containersList = [];

      for (const repo of tenantRepos) {
        try {
          const tagsResponse = await fetch(`/docker-registry/v2/${repo}/tags/list`, {
            method: 'GET',
            headers: authHeaders,
          });

          if (tagsResponse.ok) {
            const tagsData = await tagsResponse.json();
            const fullName = tagsData.name || repo;
            const rawTags = tagsData.tags || [];

            // Only include containers that have tags
            if (rawTags.length > 0) {
              // Sort tags in descending order (newest first)
              const sortedTags = sortTags(rawTags);

              // Store full registry name for API calls, display name without prefix
              const displayName = fullName.startsWith(tenantPrefix)
                ? fullName.slice(tenantPrefix.length)
                : fullName;

              containersList.push({
                name: fullName,
                displayName,
                tags: sortedTags,
              });
            }
          }
        } catch (err) {
          console.warn(`Failed to fetch tags for ${repo}:`, err);
        }
      }

      setContainers(containersList);
      
      // Update selected tags after fetching all containers
      setSelectedTags(prev => {
        const updated = { ...prev };
        containersList.forEach(container => {
          if (container.tags && container.tags.length > 0) {
            const currentSelected = prev[container.name];
            if (!currentSelected || !container.tags.includes(currentSelected)) {
              updated[container.name] = container.tags[0]; // Select newest tag
            } else {
              updated[container.name] = currentSelected; // Keep current selection
            }
          } else {
            delete updated[container.name]; // Remove selection if no tags
          }
        });
        return updated;
      });
    } catch (err) {
      setError(err.message || 'An error occurred while fetching containers');
    } finally {
      setLoading(false);
    }
  }, [tenantSlug]);

  // Initial load
  useEffect(() => {
    fetchContainers();
  }, [fetchContainers]);

  // Handle file selection
  const handleFileChange = (event) => {
    const file = event.target.files[0];
    if (file) {
      // Validate file type
      if (!file.name.endsWith('.tar') && !file.name.endsWith('.tar.gz')) {
        setUploadError('File must be a .tar or .tar.gz file');
        return;
      }
      setContainerFile(file);
      setUploadError(null);
    }
  };

  // Handle delete tag
  const handleDelete = async (name, tag) => {
    setDeleting({ [`${name}:${tag}`]: true });
    setError(null);
    
    try {
      const myHeaders = new Headers();
      myHeaders.append('Authorization', localStorage.getItem('token'));
      if (tenantSlug) myHeaders.append('X-Tenant-Slug', tenantSlug);

      const response = await fetch(
        `/api/containers/delete?name=${encodeURIComponent(name)}&tag=${encodeURIComponent(tag)}`,
        {
          method: 'DELETE',
          headers: myHeaders,
        }
      );

      if (response.status === 401) {
        router.push('/auth/login');
        return;
      }

      const result = await response.json();

      if (response.status === 200) {
        setSuccess(`Container ${name}:${tag} deleted successfully`);
        
        // Update selected tag: find remaining tags and select next one
        const container = containers.find(c => c.name === name);
        if (container) {
          const remainingTags = container.tags.filter(t => t !== tag);
          const sortedRemaining = sortTags(remainingTags);
          
          setSelectedTags(prev => {
            const updated = { ...prev };
            if (sortedRemaining.length > 0) {
              updated[name] = sortedRemaining[0]; // Select newest remaining tag
            } else {
              delete updated[name]; // No tags left
            }
            return updated;
          });
        }
        
        // Refresh containers list
        setTimeout(() => {
          fetchContainers();
        }, 1000);
      } else {
        setError(result.error || result.message || 'Failed to delete container');
      }
    } catch (err) {
      setError(err.message || 'An error occurred while deleting container');
    } finally {
      setDeleting({ [`${name}:${tag}`]: false });
      setDeleteConfirm(null);
    }
  };

  // Handle delete container (all tags)
  const handleDeleteContainer = async (name) => {
    const container = containers.find(c => c.name === name);
    if (!container || !container.tags || container.tags.length === 0) {
      return;
    }

    setDeleting({ [`${name}:all`]: true });
    setError(null);
    
    try {
      const myHeaders = new Headers();
      myHeaders.append('Authorization', localStorage.getItem('token'));
      if (tenantSlug) myHeaders.append('X-Tenant-Slug', tenantSlug);

      // Delete all tags sequentially
      const deletePromises = container.tags.map(tag =>
        fetch(
          `/api/containers/delete?name=${encodeURIComponent(name)}&tag=${encodeURIComponent(tag)}`,
          {
            method: 'DELETE',
            headers: myHeaders,
          }
        )
      );

      const responses = await Promise.all(deletePromises);
      const failed = responses.filter(r => r.status !== 200);

      if (failed.length === 0) {
        setSuccess(`Container ${name} and all tags deleted successfully`);
        
        // Remove selection
        setSelectedTags(prev => {
          const updated = { ...prev };
          delete updated[name];
          return updated;
        });
        
        // Refresh containers list
        setTimeout(() => {
          fetchContainers();
        }, 1000);
      } else {
        setError(`Failed to delete some tags from container ${name}`);
      }
    } catch (err) {
      setError(err.message || 'An error occurred while deleting container');
    } finally {
      setDeleting({ [`${name}:all`]: false });
      setDeleteContainerConfirm(null);
      setMenuAnchor(null);
      setMenuContainer(null);
    }
  };

  // Validate tag uniqueness
  const validateTag = (containerName, tag) => {
    if (!containerName || !tag) {
      setTagError(null);
      return true;
    }

    const container = containers.find(c => c.name === containerName);
    if (container && container.tags && container.tags.includes(tag)) {
      setTagError(`Tag "${tag}" already exists for this container`);
      return false;
    }
    
    setTagError(null);
    return true;
  };

  // Get current container name (either selected or custom)
  const getCurrentContainerName = () => {
    if (selectedContainerOption === 'new') {
      return customContainerName.trim();
    }
    return selectedContainerOption;
  };

  // Handle upload
  const handleUpload = async () => {
    const containerName = getCurrentContainerName();
    
    if (!containerName) {
      setUploadError('Container name is required');
      return;
    }
    if (!containerTag.trim()) {
      setUploadError('Tag is required');
      return;
    }
    if (!validateTag(containerName, containerTag.trim())) {
      return; // Tag validation error already set
    }
    if (!containerFile) {
      setUploadError('Container file is required');
      return;
    }

    setUploading(true);
    setUploadError(null);
    setTagError(null);

    try {
      const formData = new FormData();
      formData.append('name', containerName);
      formData.append('tag', containerTag.trim());
      formData.append('file', containerFile);

      const myHeaders = new Headers();
      myHeaders.append('Authorization', localStorage.getItem('token'));
      if (tenantSlug) myHeaders.append('X-Tenant-Slug', tenantSlug);
      // Don't set Content-Type for FormData - browser will set it with boundary

      const response = await fetch(
        '/api/containers/upload',
        {
          method: 'POST',
          headers: myHeaders,
          body: formData,
        }
      );

      if (response.status === 401) {
        router.push('/auth/login');
        return;
      }

      const result = await response.json();

      if (response.status === 200) {
        setSuccess('Container uploaded and pushed to registry successfully');
        setShowUploadDialog(false);
        setSelectedContainerOption('');
        setCustomContainerName('');
        setContainerTag('');
        setContainerFile(null);
        setTagError(null);
        // Refresh containers list
        setTimeout(() => {
          fetchContainers();
        }, 2000);
      } else {
        setUploadError(result.message || result || 'Failed to upload container');
      }
    } catch (err) {
      setUploadError(err.message || 'An error occurred while uploading container');
    } finally {
      setUploading(false);
    }
  };

  return (
    <>
      <Head>
        <title>
          Containers Store | Oktopus
        </title>
      </Head>
      <Box
        component="main"
        sx={{
          flexGrow: 1,
          py: 8
        }}
      >
        <Container maxWidth="xl">
          <Stack spacing={3}>
            <Stack
              direction="row"
              justifyContent="space-between"
              spacing={4}
            >
              <Typography variant="h4">
                Containers Store
              </Typography>
              <Button
                startIcon={(
                  <SvgIcon fontSize="small">
                    <PlusIcon />
                  </SvgIcon>
                )}
                onClick={() => {
                  setSelectedContainerOption('new');
                  setCustomContainerName('');
                  setContainerTag('');
                  setContainerFile(null);
                  setUploadError(null);
                  setTagError(null);
                  setShowUploadDialog(true);
                }}
                variant="contained"
              >
                Upload Container
              </Button>
            </Stack>

            <Alert severity="warning" sx={{ mb: 2 }}>
              Registry server is currently exposed. Any tenant with network access can upload containers directly to other tenants' namespaces. This will be addressed in a future update.
            </Alert>

            {error && (
              <Alert severity="error" onClose={() => setError(null)}>
                {error}
              </Alert>
            )}

            {success && (
              <Alert severity="success" onClose={() => setSuccess(null)}>
                {success}
              </Alert>
            )}

            <Card>
              <CardHeader title="Available Containers" />
              <CardContent>
                {loading ? (
                  <Box display="flex" justifyContent="center" p={3}>
                    <CircularProgress />
                  </Box>
                ) : containers.length === 0 ? (
                  <Box display="flex" justifyContent="center" p={3}>
                    <Typography color="text.secondary">
                      No containers found in registry
                    </Typography>
                  </Box>
                ) : (
                  <TableContainer component={Paper}>
                    <Table>
                      <TableHead>
                        <TableRow>
                          <TableCell>Container Name</TableCell>
                          <TableCell>Tags</TableCell>
                          <TableCell align="right">Actions</TableCell>
                        </TableRow>
                      </TableHead>
                      <TableBody>
                        {containers.map((container, index) => {
                          const selectedTag = selectedTags[container.name];
                          const hasTags = container.tags && container.tags.length > 0;
                          const isDeletingTag = hasTags && deleting[`${container.name}:${selectedTag}`];
                          const isDeletingContainer = deleting[`${container.name}:all`];
                          
                          return (
                            <TableRow key={index}>
                              <TableCell>{container.displayName || container.name}</TableCell>
                              <TableCell>
                                {hasTags ? (
                                  <Select
                                    value={selectedTag || ''}
                                    onChange={(e) => {
                                      setSelectedTags(prev => ({
                                        ...prev,
                                        [container.name]: e.target.value
                                      }));
                                    }}
                                    size="small"
                                    sx={{ minWidth: 120 }}
                                    disabled={isDeletingTag || isDeletingContainer}
                                  >
                                    {container.tags.map((tag) => (
                                      <MenuItem key={tag} value={tag}>
                                        {tag}
                                      </MenuItem>
                                    ))}
                                  </Select>
                                ) : (
                                  <Typography color="text.secondary" variant="body2">
                                    No tags
                                  </Typography>
                                )}
                              </TableCell>
                              <TableCell align="right">
                                <Stack direction="row" spacing={1} justifyContent="flex-end">
                                  <Tooltip title="Upload new tag">
                                    <IconButton
                                      size="small"
                                      onClick={() => {
                                        setSelectedContainerOption(container.name);
                                        setCustomContainerName('');
                                        setContainerTag('');
                                        setContainerFile(null);
                                        setUploadError(null);
                                        setTagError(null);
                                        setShowUploadDialog(true);
                                      }}
                                      disabled={isDeletingTag || isDeletingContainer}
                                      sx={{
                                        color: 'primary.main',
                                        '&:hover': {
                                          backgroundColor: 'error.light',
                                          color: 'error.dark',
                                        },
                                      }}
                                    >
                                      <SvgIcon fontSize="small">
                                        <ArrowUpTrayIcon />
                                      </SvgIcon>
                                    </IconButton>
                                  </Tooltip>
                                  {hasTags ? (
                                    <>
                                      <Tooltip title="Delete selected tag">
                                        <IconButton
                                          size="small"
                                          onClick={() => setDeleteConfirm({ name: container.name, tag: selectedTag })}
                                          disabled={isDeletingTag || isDeletingContainer || !selectedTag}
                                          sx={{
                                            color: 'error.main',
                                            '&:hover': {
                                              backgroundColor: 'error.light',
                                              color: 'error.dark',
                                            },
                                          }}
                                        >
                                          <SvgIcon fontSize="small">
                                            <TrashIcon />
                                          </SvgIcon>
                                        </IconButton>
                                      </Tooltip>
                                      <Tooltip title="Container actions">
                                        <IconButton
                                          size="small"
                                          onClick={(e) => {
                                            setMenuAnchor(e.currentTarget);
                                            setMenuContainer(container);
                                          }}
                                          disabled={isDeletingTag || isDeletingContainer}
                                        >
                                          <SvgIcon fontSize="small">
                                            <EllipsisVerticalIcon />
                                          </SvgIcon>
                                        </IconButton>
                                      </Tooltip>
                                    </>
                                  ) : (
                                    <Tooltip title="Delete container">
                                      <IconButton
                                        size="small"
                                        onClick={() => setDeleteContainerConfirm({ name: container.name })}
                                        disabled={isDeletingContainer}
                                        sx={{
                                          color: 'error.main',
                                          '&:hover': {
                                            backgroundColor: 'error.light',
                                            color: 'error.dark',
                                          },
                                        }}
                                      >
                                        <SvgIcon fontSize="small">
                                          <TrashIcon />
                                        </SvgIcon>
                                      </IconButton>
                                    </Tooltip>
                                  )}
                                </Stack>
                              </TableCell>
                            </TableRow>
                          );
                        })}
                      </TableBody>
                    </Table>
                  </TableContainer>
                )}
              </CardContent>
            </Card>
          </Stack>
        </Container>
      </Box>

      {/* Upload Dialog */}
      <Dialog
        open={showUploadDialog}
        onClose={() => {
          if (!uploading) {
            setShowUploadDialog(false);
            setSelectedContainerOption('');
            setCustomContainerName('');
            setContainerTag('');
            setContainerFile(null);
            setUploadError(null);
            setTagError(null);
          }
        }}
        maxWidth="sm"
        fullWidth
      >
        <DialogTitle>
          <Box display="flex" justifyContent="space-between" alignItems="center">
            <Typography variant="h6">Upload Container to Registry</Typography>
            <IconButton
              onClick={() => {
                if (!uploading) {
                  setShowUploadDialog(false);
                  setSelectedContainerOption('');
                  setCustomContainerName('');
                  setContainerTag('');
                  setContainerFile(null);
                  setUploadError(null);
                  setTagError(null);
                }
              }}
              disabled={uploading}
            >
              <SvgIcon>
                <XMarkIcon />
              </SvgIcon>
            </IconButton>
          </Box>
        </DialogTitle>
        <DialogContent>
          <Stack spacing={3} mt={1}>
            {uploadError && (
              <Alert severity="error" onClose={() => setUploadError(null)}>
                {uploadError}
              </Alert>
            )}

            {/* Container selection dropdown */}
            <FormControl fullWidth>
              <InputLabel id="container-select-label">Container</InputLabel>
              <Select
                labelId="container-select-label"
                value={selectedContainerOption}
                onChange={(e) => {
                  const value = e.target.value;
                  setSelectedContainerOption(value);
                  setCustomContainerName('');
                  setContainerTag('');
                  setTagError(null);
                }}
                disabled={uploading}
                label="Container"
              >
                <MenuItem value="new">
                  <em>New Container</em>
                </MenuItem>
                {containers.map((container) => (
                  <MenuItem key={container.name} value={container.name}>
                    {container.displayName || container.name}
                  </MenuItem>
                ))}
              </Select>
            </FormControl>

            {/* Custom container name input (shown only when "New Container" is selected) */}
            {selectedContainerOption === 'new' && (
              <TextField
                label="New Container Name"
                variant="outlined"
                fullWidth
                required
                value={customContainerName}
                onChange={(e) => {
                  setCustomContainerName(e.target.value);
                  // Re-validate tag when container name changes
                  if (containerTag) {
                    validateTag(e.target.value.trim(), containerTag.trim());
                  }
                }}
                disabled={uploading}
                placeholder="e.g., my_cortexa53_container"
                helperText="Name for the new container"
                name="container-name"
                id="container-name"
              />
            )}

            {/* Tag input */}
            <TextField
              label="Tag"
              variant="outlined"
              fullWidth
              required
              value={containerTag}
              onChange={(e) => {
                setContainerTag(e.target.value);
                // Validate tag uniqueness
                const containerName = getCurrentContainerName();
                validateTag(containerName, e.target.value.trim());
              }}
              disabled={uploading || !selectedContainerOption}
              placeholder="e.g., v0.0.1"
              helperText={tagError || "Version tag for the container"}
              error={!!tagError}
              name="container-tag"
              id="container-tag"
            />

            <Box>
              <Typography variant="subtitle2" gutterBottom>
                Container File (.tar or .tar.gz)
              </Typography>
              <Button
                variant="outlined"
                component="label"
                disabled={uploading}
                fullWidth
              >
                {containerFile ? containerFile.name : 'Select File'}
                <input
                  type="file"
                  hidden
                  accept=".tar,.tar.gz"
                  onChange={handleFileChange}
                />
              </Button>
              {containerFile && (
                <Typography variant="caption" color="text.secondary" sx={{ mt: 1, display: 'block' }}>
                  Selected: {containerFile.name} ({(containerFile.size / 1024 / 1024).toFixed(2)} MB)
                </Typography>
              )}
            </Box>
          </Stack>
        </DialogContent>
        <DialogActions>
          <Button
            onClick={() => {
              if (!uploading) {
                setShowUploadDialog(false);
                setSelectedContainerOption('');
                setCustomContainerName('');
                setContainerTag('');
                setContainerFile(null);
                setUploadError(null);
                setTagError(null);
              }
            }}
            disabled={uploading}
          >
            Cancel
          </Button>
          <Button
            onClick={handleUpload}
            variant="contained"
            disabled={
              uploading || 
              !selectedContainerOption || 
              (selectedContainerOption === 'new' && !customContainerName.trim()) ||
              !containerTag.trim() || 
              !containerFile ||
              !!tagError
            }
          >
            {uploading ? <CircularProgress size={20} /> : 'Upload'}
          </Button>
        </DialogActions>
      </Dialog>

      {/* Delete Tag Confirmation Dialog */}
      <Dialog
        open={deleteConfirm !== null}
        onClose={() => setDeleteConfirm(null)}
        maxWidth="sm"
        fullWidth
      >
        <DialogTitle>Confirm Delete Tag</DialogTitle>
        <DialogContent>
          <Typography>
            Are you sure you want to delete <strong>{deleteConfirm?.name}:{deleteConfirm?.tag}</strong> from the registry?
          </Typography>
          <Typography variant="body2" color="text.secondary" sx={{ mt: 1 }}>
            This action cannot be undone.
          </Typography>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setDeleteConfirm(null)} disabled={deleting[`${deleteConfirm?.name}:${deleteConfirm?.tag}`]}>
            Cancel
          </Button>
          <Button
            onClick={() => handleDelete(deleteConfirm?.name, deleteConfirm?.tag)}
            variant="contained"
            color="error"
            disabled={deleting[`${deleteConfirm?.name}:${deleteConfirm?.tag}`]}
          >
            {deleting[`${deleteConfirm?.name}:${deleteConfirm?.tag}`] ? (
              <CircularProgress size={20} />
            ) : (
              'Delete'
            )}
          </Button>
        </DialogActions>
      </Dialog>

      {/* Delete Container Confirmation Dialog */}
      <Dialog
        open={deleteContainerConfirm !== null}
        onClose={() => setDeleteContainerConfirm(null)}
        maxWidth="sm"
        fullWidth
      >
        <DialogTitle>Confirm Delete Container</DialogTitle>
        <DialogContent>
          <Typography>
            Are you sure you want to delete container <strong>{deleteContainerConfirm?.name}</strong> and all its tags from the registry?
          </Typography>
          <Typography variant="body2" color="text.secondary" sx={{ mt: 1 }}>
            This will delete all tags for this container. This action cannot be undone.
          </Typography>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setDeleteContainerConfirm(null)} disabled={deleting[`${deleteContainerConfirm?.name}:all`]}>
            Cancel
          </Button>
          <Button
            onClick={() => handleDeleteContainer(deleteContainerConfirm?.name)}
            variant="contained"
            color="error"
            disabled={deleting[`${deleteContainerConfirm?.name}:all`]}
          >
            {deleting[`${deleteContainerConfirm?.name}:all`] ? (
              <CircularProgress size={20} />
            ) : (
              'Delete Container'
            )}
          </Button>
        </DialogActions>
      </Dialog>

      {/* Container Actions Menu */}
      <Menu
        anchorEl={menuAnchor}
        open={Boolean(menuAnchor)}
        onClose={() => {
          setMenuAnchor(null);
          setMenuContainer(null);
        }}
      >
        <MenuItem
          onClick={() => {
            if (menuContainer) {
              setDeleteContainerConfirm({ name: menuContainer.name });
            }
            setMenuAnchor(null);
            setMenuContainer(null);
          }}
          disabled={menuContainer && deleting[`${menuContainer.name}:all`]}
        >
          Delete Container
        </MenuItem>
      </Menu>
    </>
  );
};

Page.getLayout = (page) => (
  <DashboardLayout>
    {page}
  </DashboardLayout>
);

export default Page;

