import { useEffect, useState, useCallback, useMemo, useRef } from 'react';
import {
  Card,
  CardContent,
  SvgIcon,
  IconButton,
  List,
  ListItemText,
  ListItemButton,
  Box,
  TextField,
  Backdrop,
  Alert,
  Typography,
  Tooltip,
  Popover,
  Grid,
  Paper,
  Chip,
  InputAdornment,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  OutlinedInput,
} from '@mui/material';
import CircularProgress from '@mui/material/CircularProgress';
import ArrowPathIcon from '@heroicons/react/24/outline/ArrowPathIcon';
import Pencil from '@heroicons/react/24/outline/PencilIcon';
import XMarkIcon from '@heroicons/react/24/outline/XMarkIcon';
import CheckIcon from '@heroicons/react/24/outline/CheckIcon';
import MagnifyingGlassIcon from '@heroicons/react/24/outline/MagnifyingGlassIcon';
import { useRouter } from 'next/router';
import { useBackendContext } from 'src/contexts/backend-context';
import { useTheme } from '@mui/material/styles';

const ChevronRightIcon = (props) => (
  <svg xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" strokeWidth={2.5} stroke="currentColor" {...props}>
    <path strokeLinecap="round" strokeLinejoin="round" d="M8.25 4.5l7.5 7.5-7.5 7.5" />
  </svg>
);

const ChevronDownIcon = (props) => (
  <svg xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" strokeWidth={2.5} stroke="currentColor" {...props}>
    <path strokeLinecap="round" strokeLinejoin="round" d="M19.5 8.25l-7.5 7.5-7.5-7.5" />
  </svg>
);

const pathToUrl = (path) => path.replace(/\.$/, '').replaceAll('.', '/');
const segmentsToPath = (segments) => (segments.length > 0 ? segments.join('.') + '.' : '');

const getDisplayName = (fullPath, parentPath) => {
  if (fullPath.startsWith(parentPath)) {
    const rel = fullPath.slice(parentPath.length);
    return rel.replace(/\.$/, '').split('.').filter(Boolean).pop() || rel;
  }
  const parts = fullPath.replace(/\.$/, '').split('.');
  return parts[parts.length - 1] || fullPath;
};

const VALUE_TRUNCATE_LENGTH = 40;

const ValueDisplay = ({ value }) => {
  const [anchorEl, setAnchorEl] = useState(null);
  const displayValue = value == null ? '-' : String(value);
  const isTruncated = displayValue.length > VALUE_TRUNCATE_LENGTH;

  return (
    <>
      <Typography
        variant="body2"
        color="text.secondary"
        onClick={isTruncated ? (e) => setAnchorEl(e.currentTarget) : undefined}
        sx={{
          maxWidth: 300,
          overflow: 'hidden',
          textOverflow: 'ellipsis',
          whiteSpace: 'nowrap',
          cursor: isTruncated ? 'pointer' : 'default',
          fontWeight: 500,
          '&:hover': isTruncated ? { color: 'primary.main' } : {},
        }}
      >
        {displayValue}
      </Typography>
      <Popover
        open={Boolean(anchorEl)}
        anchorEl={anchorEl}
        onClose={() => setAnchorEl(null)}
        anchorOrigin={{ vertical: 'bottom', horizontal: 'right' }}
        transformOrigin={{ vertical: 'top', horizontal: 'right' }}
      >
        <Box sx={{ p: 2, maxWidth: 500, maxHeight: 300, overflow: 'auto' }}>
          <Typography
            variant="body2"
            sx={{ wordBreak: 'break-all', whiteSpace: 'pre-wrap', userSelect: 'all', fontWeight: 500 }}
          >
            {displayValue}
          </Typography>
        </Box>
      </Popover>
    </>
  );
};

export const DevicesDiscovery = ({ onStatusRefresh }) => {
  const router = useRouter();
  const theme = useTheme();
  const { httpRequest, apiPrefix } = useBackendContext();

  const deviceID = router.query.id?.[0];
  const pathSegments = useMemo(() => router.query.id?.slice(2) || [], [router.query.id]);
  const pathKey = pathSegments.join('/');
  const currentPath = useMemo(() => segmentsToPath(pathSegments), [pathSegments]);

  // Tree and Schema state
  const [rootPath, setRootPath] = useState(null);
  const [dataModel, setDataModel] = useState('');
  const [treeNodes, setTreeNodes] = useState({});

  // Original states for Right Details Panel
  const [children, setChildren] = useState([]);
  const [parameters, setParameters] = useState([]);
  const [showLoading, setShowLoading] = useState(false);
  const [errorText, setErrorText] = useState('');

  // Inline Parameter Editing State
  const [editingParam, setEditingParam] = useState(null); // param name being edited
  const [editingValue, setEditingValue] = useState('');

  // Live in-place Tree Search State
  const [treeSearchQuery, setTreeSearchQuery] = useState('');
  const searchRunRef = useRef(0);

  // Navigate to a path by updating Next.js URL
  const navigateTo = useCallback((path) => {
    const urlPath = pathToUrl(path);
    router.push(
      `/devices/cwmp/${deviceID}/discovery/${urlPath}`,
      undefined,
      { shallow: true }
    );
  }, [deviceID, router]);

  // Navigate up one level
  const navigateBack = useCallback(() => {
    const parts = currentPath.replace(/\.$/, '').split('.');
    if (parts.length <= 1) return;
    parts.pop();
    navigateTo(parts.join('.') + '.');
  }, [currentPath, navigateTo]);

  // Recursive ancestor populator for smooth tree expansion on deep load or search jump
  const ensurePathInTree = useCallback((targetPath) => {
    if (!rootPath) return;
    const cleanTarget = targetPath.startsWith(rootPath) ? targetPath : rootPath;
    const parts = cleanTarget.replace(/\.$/, '').split('.');
    const ancestors = [];
    for (let i = 1; i <= parts.length; i++) {
      ancestors.push(parts.slice(0, i).join('.') + '.');
    }

    setTreeNodes(prev => {
      let updated = { ...prev };
      ancestors.forEach((anc, idx) => {
        if (!updated[anc]) {
          updated[anc] = {
            path: anc,
            name: parts[idx] || anc,
            expanded: true,
            loaded: false,
            loading: false,
            children: [],
          };
        } else {
          // If searching or deep loading, expand ancestors
          updated[anc] = {
            ...updated[anc],
            expanded: true,
          };
        }

        // Link parent to this child if not already linked
        if (idx > 0) {
          const parentAnc = ancestors[idx - 1];
          if (updated[parentAnc] && !updated[parentAnc].children.includes(anc)) {
            updated[parentAnc] = {
              ...updated[parentAnc],
              children: [...updated[parentAnc].children, anc].sort(),
            };
          }
        }
      });
      return updated;
    });
  }, [rootPath]);

  // Main data loader function
  const loadPath = useCallback(async (path, { preserveState = false } = {}) => {
    if (!deviceID || !path) return;
    onStatusRefresh?.();
    setShowLoading(true);
    if (!preserveState) {
      setErrorText('');
      setChildren([]);
      setParameters([]);
    }
    try {
      const body = JSON.stringify({ path, fetch_values: true });
      const { status, result } = await httpRequest(
        `${apiPrefix}/device/cwmp/${deviceID}/parameters`,
        'PUT',
        body
      );
      if (status === 200 && result) {
        const childList = result.children || [];
        const paramList = result.parameters || [];

        setChildren(childList);
        setParameters(paramList);
        if (result.data_model) setDataModel(result.data_model);

        // Update treeNodes state dynamically
        setTreeNodes(prev => {
          let newTreeNodes = { ...prev };
          const childPaths = childList.map(c => c.name);

          childPaths.forEach(childPath => {
            if (!newTreeNodes[childPath]) {
              newTreeNodes[childPath] = {
                path: childPath,
                name: getDisplayName(childPath, path),
                expanded: false,
                loaded: false,
                loading: false,
                children: [],
              };
            }
          });

          newTreeNodes[path] = {
            ...newTreeNodes[path],
            loaded: true,
            loading: false,
            children: childPaths,
          };

          return newTreeNodes;
        });

        return;
      }
      setErrorText('Failed to load parameters from device');
    } catch {
      setErrorText('Failed to load parameters from device');
    } finally {
      setShowLoading(false);
    }
  }, [apiPrefix, deviceID, httpRequest, onStatusRefresh]);

  const loadTreePathForSearch = useCallback(async (path) => {
    if (!deviceID || !path) return [];

    try {
      const body = JSON.stringify({ path, fetch_values: false });
      const { status, result } = await httpRequest(
        `${apiPrefix}/device/cwmp/${deviceID}/parameters`,
        'PUT',
        body
      );

      if (status !== 200 || !result) return [];

      const childPaths = (result.children || []).map((child) => child.name);
      setTreeNodes(prev => {
        const next = { ...prev };
        childPaths.forEach(childPath => {
          if (!next[childPath]) {
            next[childPath] = {
              path: childPath,
              name: getDisplayName(childPath, path),
              expanded: false,
              loaded: false,
              loading: false,
              children: [],
            };
          }
        });

        next[path] = {
          ...next[path],
          path,
          loaded: true,
          loading: false,
          children: childPaths,
        };

        return next;
      });

      return childPaths;
    } catch {
      return [];
    }
  }, [apiPrefix, deviceID, httpRequest]);

  // Initial root detection on mount
  useEffect(() => {
    if (!deviceID) return;

    const init = async () => {
      if (!pathSegments.length) {
        const { status, result } = await httpRequest(
          `${apiPrefix}/device/cwmp/${deviceID}/root`,
          'GET'
        );
        if (status === 200 && result?.path) {
          setRootPath(result.path);
          setDataModel(result.data_model || '');
          router.replace(
            `/devices/cwmp/${deviceID}/discovery/${pathToUrl(result.path)}`,
            undefined,
            { shallow: true }
          );
          return;
        }
        setErrorText('Could not detect CWMP data model root');
        return;
      }
      if (!rootPath && currentPath) {
        if (currentPath.startsWith('InternetGatewayDevice.')) {
          setRootPath('InternetGatewayDevice.');
          setDataModel('TR098');
        } else {
          setRootPath('Device.');
          setDataModel('TR181');
        }
      }
    };

    init();
  }, [apiPrefix, currentPath, deviceID, httpRequest, pathSegments.length, rootPath, router]);

  // Keep tree nodes state aligned with the active root path
  useEffect(() => {
    if (rootPath) {
      setTreeNodes(prev => {
        if (prev[rootPath]) return prev;
        return {
          [rootPath]: {
            path: rootPath,
            name: rootPath.replace(/\.$/, ''),
            expanded: true,
            loaded: false,
            loading: false,
            children: [],
          }
        };
      });
    }
  }, [rootPath]);

  useEffect(() => {
    const query = treeSearchQuery.trim();
    if (!rootPath || query.length < 2) return undefined;

    const runId = searchRunRef.current + 1;
    searchRunRef.current = runId;
    const visited = new Set();

    const crawl = async (paths, depth = 0) => {
      if (searchRunRef.current !== runId || depth > 6) return;

      for (const path of paths) {
        if (searchRunRef.current !== runId || visited.has(path)) continue;
        visited.add(path);

        const childPaths = await loadTreePathForSearch(path);
        if (childPaths.length > 0) {
          await crawl(childPaths, depth + 1);
        }
      }
    };

    const timer = setTimeout(() => {
      crawl([rootPath]);
    }, 300);

    return () => {
      clearTimeout(timer);
      searchRunRef.current += 1;
    };
  }, [treeSearchQuery, rootPath, loadTreePathForSearch]);

  // Handle URL synchronizations and data fetching
  useEffect(() => {
    if (!deviceID) return;
    const path = currentPath || rootPath;
    if (path) {
      ensurePathInTree(path);
      loadPath(path);
    }
  }, [currentPath, deviceID, rootPath, loadPath, ensurePathInTree]);

  // Inline Param editing submission
  const applyEdit = async (paramName, paramValue) => {
    setEditingParam(null);
    setShowLoading(true);
    try {
      const body = JSON.stringify({ values: { [paramName]: paramValue } });
      const { status } = await httpRequest(
        `${apiPrefix}/device/cwmp/${deviceID}/set`,
        'PUT',
        body
      );
      if (status === 200) {
        await loadPath(currentPath || rootPath);
      } else {
        setErrorText('Failed to set parameter value');
      }
    } catch {
      setErrorText('Failed to set parameter value');
    } finally {
      setShowLoading(false);
    }
  };

  // Toggle tree node expanded state
  const handleToggleNode = useCallback((path) => {
    setTreeNodes(prev => {
      const node = prev[path];
      if (!node) return prev;

      const nextExpanded = !node.expanded;
      if (nextExpanded && !node.loaded) {
        setTimeout(() => loadPath(path), 0);
        return {
          ...prev,
          [path]: { ...node, expanded: true, loading: true }
        };
      }

      return {
        ...prev,
        [path]: { ...node, expanded: nextExpanded }
      };
    });
  }, [loadPath]);

  // Recursive Tree Filtering check for In-Place search
  const shouldShowNode = useCallback((path, query) => {
    if (!query) return true;
    const q = query.toLowerCase();
    if (path.toLowerCase().includes(q)) return true;

    const node = treeNodes[path];
    if (!node || !node.children) return false;
    return node.children.some(childPath => shouldShowNode(childPath, query));
  }, [treeNodes]);

  // Rendering Left Directory Tree nodes with automatic filter
  const renderTreeList = (nodePaths) => {
    if (!nodePaths || nodePaths.length === 0) return null;

    // Filter paths based on our in-place search query
    const filteredPaths = nodePaths.filter(path => shouldShowNode(path, treeSearchQuery));
    const sorted = [...filteredPaths].sort();

    return (
      <List dense disablePadding>
        {sorted.map(path => {
          const node = treeNodes[path];
          if (!node) return null;

          const isSelected = (currentPath || rootPath) === path;
          const isExpandable = path.endsWith('.');

          // Auto expand if there is an active search query and this node contains a match
          const hasChildren = node.children && node.children.length > 0;
          const isSearchActive = !!treeSearchQuery;
          const shouldExpand = isSearchActive
            ? (node.children.some(childPath => shouldShowNode(childPath, treeSearchQuery)))
            : node.expanded;

          return (
            <Box key={path} id={`tree-node-${path.replaceAll('.', '_')}`}>
              <ListItemButton
                selected={isSelected}
                onClick={() => navigateTo(path)}
                sx={{
                  pl: (path.split('.').length - 2) * 1.5 + 1.5,
                  py: 0.5,
                  my: 0.2,
                  borderRadius: '6px',
                  border: isSelected ? `1px solid ${theme.palette.primary.main}40` : '1px solid transparent',
                  '&.Mui-selected': {
                    bgcolor: 'primary.alpha10',
                    color: 'primary.main',
                    fontWeight: 600,
                    '&:hover': { bgcolor: 'primary.alpha15' }
                  },
                }}
              >
                {isExpandable && (
                  <IconButton
                    size="small"
                    onClick={(e) => {
                      e.stopPropagation();
                      handleToggleNode(path);
                    }}
                    sx={{ p: 0.2, mr: 0.5, color: isSelected ? 'primary.main' : 'text.secondary' }}
                  >
                    {node.loading ? (
                      <CircularProgress size={14} color="inherit" />
                    ) : shouldExpand ? (
                      <ChevronDownIcon style={{ width: 14, height: 14 }} />
                    ) : (
                      <ChevronRightIcon style={{ width: 14, height: 14 }} />
                    )}
                  </IconButton>
                )}
                {!isExpandable && <Box sx={{ width: 22 }} />}
                <ListItemText
                  primary={node.name + '.'}
                  primaryTypographyProps={{
                    variant: 'body2',
                    fontWeight: isSelected ? 600 : 500,
                    sx: { wordBreak: 'break-all', userSelect: 'none' }
                  }}
                />
              </ListItemButton>
              {shouldExpand && hasChildren && (
                <Box>
                  {renderTreeList(node.children)}
                </Box>
              )}
            </Box>
          );
        })}
      </List>
    );
  };

  // Rendering clickable Right Pane Breadcrumbs
  const renderBreadcrumbs = () => {
    const browsePath = currentPath || rootPath || '';
    const segments = browsePath.replace(/\.$/, '').split('.');
    return (
      <Box sx={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center', gap: 0.5, py: 0.5 }}>
        {segments.map((seg, idx) => {
          const isLast = idx === segments.length - 1;
          const targetPath = segments.slice(0, idx + 1).join('.') + '.';
          return (
            <Box key={idx} sx={{ display: 'flex', alignItems: 'center' }}>
              {idx > 0 && <Typography color="text.secondary" sx={{ mx: 0.5, fontWeight: 700 }}>·</Typography>}
              {isLast ? (
                <Typography variant="subtitle1" sx={{ fontWeight: 700, color: 'text.primary' }}>
                  {seg}
                </Typography>
              ) : (
                <Typography
                  variant="subtitle1"
                  onClick={() => navigateTo(targetPath)}
                  sx={{
                    fontWeight: 500,
                    color: 'primary.main',
                    cursor: 'pointer',
                    '&:hover': { textDecoration: 'underline' },
                  }}
                >
                  {seg}
                </Typography>
              )}
            </Box>
          );
        })}
        <Typography variant="subtitle1" sx={{ fontWeight: 700 }}>.</Typography>
      </Box>
    );
  };

  const browsePath = currentPath || rootPath || '';

  const renderLoading = () => (
    <Box sx={{ display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', py: 8, gap: 2 }}>
      <CircularProgress size={40} />
      <Typography variant="body2" color="text.secondary">Loading parameters...</Typography>
    </Box>
  );

  return (
    <Card sx={{ border: '1px solid', borderColor: 'divider', boxShadow: theme.shadows[4] }}>
      <CardContent sx={{ p: 0, '&:last-child': { pb: 0 } }}>
        <Grid container sx={{ minHeight: '780px' }}>

          {/* LEFT COLUMN: Collapsible Directory Tree */}
          <Grid item xs={12} md={4.5} sx={{ borderRight: '1px solid', borderColor: 'divider', display: 'flex', flexDirection: 'column' }}>
            <Box sx={{ p: 2, borderBottom: '1px solid', borderColor: 'divider', bgcolor: 'background.neutral', display: 'flex', alignItems: 'center' }}>
              <OutlinedInput
                fullWidth
                placeholder="Search TR-181 nodes..."
                value={treeSearchQuery}
                onChange={(e) => setTreeSearchQuery(e.target.value)}
                startAdornment={(
                  <InputAdornment position="start">
                    <SvgIcon
                      color="action"
                      fontSize="small"
                    >
                      <MagnifyingGlassIcon />
                    </SvgIcon>
                  </InputAdornment>
                )}
                endAdornment={treeSearchQuery && (
                  <InputAdornment position="end">
                    <IconButton size="small" onClick={() => setTreeSearchQuery('')}>
                      <SvgIcon fontSize="inherit"><XMarkIcon /></SvgIcon>
                    </IconButton>
                  </InputAdornment>
                )}
                sx={{
                  borderRadius: '8px',
                  bgcolor: 'background.paper',
                }}
              />
            </Box>

            <Box
              sx={{
                flexGrow: 1,
                p: 1.5,
                maxHeight: '780px',
                overflowY: 'auto',
                '&::-webkit-scrollbar': { width: '4px' },
                '&::-webkit-scrollbar-thumb': { bgcolor: 'divider', borderRadius: '4px' }
              }}
            >
              {rootPath ? renderTreeList([rootPath]) : (
                <Box sx={{ display: 'flex', justifyContent: 'center', py: 4 }}>
                  <CircularProgress size={20} />
                </Box>
              )}
            </Box>
          </Grid>

          {/* RIGHT COLUMN: Tabbed Active Node details */}
          <Grid item xs={12} md={7.5} sx={{ display: 'flex', flexDirection: 'column', bgcolor: 'background.paper' }}>
            {browsePath ? (
              <Box sx={{ p: 2, borderBottom: '1px solid', borderColor: 'divider', display: 'flex', alignItems: 'center', justifyContent: 'space-between', flexWrap: 'wrap', gap: 1 }}>
                {renderBreadcrumbs()}
                <Tooltip title="Refresh">
                  <IconButton onClick={() => loadPath(browsePath)} disabled={!browsePath} size="small">
                    <SvgIcon fontSize="small"><ArrowPathIcon /></SvgIcon>
                  </IconButton>
                </Tooltip>
              </Box>
            ) : null}

            {/* Details panel body */}
            <Box sx={{ p: 3, flexGrow: 1, maxHeight: '748px', overflowY: 'auto' }}>
              {errorText && (
                <Alert severity="error" sx={{ mb: 2 }} onClose={() => setErrorText('')}>
                  {errorText}
                </Alert>
              )}

              {showLoading && !parameters.length && !children.length ? renderLoading() : (
                <>
                  {(() => {
                    if (parameters.length === 0) {
                      return (
                        <Box sx={{ py: 6, textAlign: 'center' }}>
                          <Typography variant="body2" color="text.secondary">No parameters found under this node.</Typography>
                        </Box>
                      );
                    }

                    // Sorting: alphabetical normal params, counts last
                    const sortedParams = [...parameters].sort((a, b) => {
                      const aIsCount = a.name.endsWith('NumberOfEntries');
                      const bIsCount = b.name.endsWith('NumberOfEntries');
                      if (aIsCount !== bIsCount) return aIsCount ? 1 : -1;
                      return a.name.localeCompare(b.name);
                    });

                    return (
                      <TableContainer component={Paper} variant="outlined" sx={{ border: '1px solid', borderColor: 'divider' }}>
                        <Table size="small">
                          <TableHead sx={{ bgcolor: 'background.neutral' }}>
                            <TableRow>
                              <TableCell sx={{ fontWeight: 700 }}>Parameter Name</TableCell>
                              <TableCell sx={{ fontWeight: 700 }}>Writable</TableCell>
                              <TableCell sx={{ fontWeight: 700, textAlign: 'right', pr: 4 }}>Value</TableCell>
                            </TableRow>
                          </TableHead>
                          <TableBody>
                            {sortedParams.map(param => {
                              const isEditing = editingParam === param.name;
                              return (
                                <TableRow key={param.name} hover>
                                  <TableCell sx={{ py: 1, fontWeight: 500 }}>
                                    {getDisplayName(param.name, browsePath)}
                                  </TableCell>
                                  <TableCell>
                                    {param.writable ? (
                                      <Chip label="Read-Write" color="primary" size="small" sx={{ height: '20px', fontSize: '10px', fontWeight: 600 }} />
                                    ) : (
                                      <Chip label="Read-Only" variant="outlined" size="small" sx={{ height: '20px', fontSize: '10px', color: 'text.secondary' }} />
                                    )}
                                  </TableCell>
                                  <TableCell sx={{ py: 1 }}>
                                    {isEditing ? (
                                      <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'flex-end', gap: 1 }}>
                                        <TextField
                                          size="small"
                                          variant="outlined"
                                          value={editingValue}
                                          onChange={(e) => setEditingValue(e.target.value)}
                                          onKeyDown={(e) => {
                                            if (e.key === 'Enter') applyEdit(param.name, editingValue);
                                            if (e.key === 'Escape') setEditingParam(null);
                                          }}
                                          sx={{
                                            width: '200px',
                                            '& .MuiInputBase-input': { py: 0.5, fontSize: '13px' }
                                          }}
                                        />
                                        <IconButton size="small" color="primary" onClick={() => applyEdit(param.name, editingValue)}>
                                          <SvgIcon fontSize="small"><CheckIcon /></SvgIcon>
                                        </IconButton>
                                        <IconButton size="small" onClick={() => setEditingParam(null)}>
                                          <SvgIcon fontSize="small"><XMarkIcon /></SvgIcon>
                                        </IconButton>
                                      </Box>
                                    ) : (
                                      <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'flex-end', gap: 1 }}>
                                        <ValueDisplay value={param.value} />
                                        {param.writable && (
                                          <IconButton
                                            size="small"
                                            onClick={(e) => {
                                              e.stopPropagation();
                                              setEditingParam(param.name);
                                              setEditingValue(param.value ?? '');
                                            }}
                                          >
                                            <SvgIcon fontSize="inherit"><Pencil /></SvgIcon>
                                          </IconButton>
                                        )}
                                      </Box>
                                    )}
                                  </TableCell>
                                </TableRow>
                              );
                            })}
                          </TableBody>
                        </Table>
                      </TableContainer>
                    );
                  })()}
                </>
              )}
            </Box>
          </Grid>

        </Grid>
      </CardContent>

      <Backdrop open={showLoading && !parameters.length} sx={{ zIndex: (theme) => theme.zIndex.drawer + 1 }}>
        <CircularProgress color="inherit" />
      </Backdrop>
    </Card>
  );
};