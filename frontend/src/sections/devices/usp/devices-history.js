import { useState, useEffect, useCallback, useRef, useMemo } from 'react';
import {
  Card,
  CardContent,
  CardHeader,
  CardActions,
  Button,
  Stack,
  Box,
  IconButton,
  SvgIcon,
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
  Switch,
  Select,
  MenuItem,
  InputLabel,
  FormControl,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Divider,
  TextField,
} from '@mui/material';
import { useRouter } from 'next/router';
import { useBackendContext } from 'src/contexts/backend-context';
import ArrowPathIcon from '@heroicons/react/24/outline/ArrowPathIcon';
import ChevronDownIcon from '@heroicons/react/24/outline/ChevronDownIcon';
import ChevronRightIcon from '@heroicons/react/24/outline/ChevronRightIcon';
import CodeBracketIcon from '@heroicons/react/24/outline/CodeBracketIcon';
import DocumentTextIcon from '@heroicons/react/24/outline/DocumentTextIcon';

// Color mapping for message types (same color for request/response pairs)
const getMessageTypeColor = (msgType) => {
  if (!msgType) return '#9E9E9E';
  const baseType = msgType.replace(/_RESP$/, '').toUpperCase();
  const colorMap = {
    'OPERATE': '#66BB6A',
    'GET': '#42A5F5',
    'SET': '#FFA726',
    'ADD': '#AB47BC',
    'DELETE': '#EF5350',
    'NOTIFY': '#26C6DA',
    'GET_SUPPORTED_DM': '#8D6E63',
    'GET_INSTANCES': '#78909C',
  };
  return colorMap[baseType] || '#9E9E9E';
};

// Format timestamp to readable date/time
const formatTimestamp = (timestamp) => {
  if (!timestamp) return 'N/A';
  return new Date(timestamp).toLocaleString();
};

// Format time ago
const formatTimeAgo = (timestamp) => {
  if (!timestamp) return 'N/A';
  const diffMs = Date.now() - new Date(timestamp).getTime();
  const diffSecs = Math.floor(diffMs / 1000);
  const diffMins = Math.floor(diffSecs / 60);
  const diffHours = Math.floor(diffMins / 60);
  const diffDays = Math.floor(diffHours / 24);

  if (diffSecs < 60) return 'Just now';
  if (diffMins < 60) return `${diffMins} minute${diffMins !== 1 ? 's' : ''} ago`;
  if (diffHours < 24) return `${diffHours} hour${diffHours !== 1 ? 's' : ''} ago`;
  return `${diffDays} day${diffDays !== 1 ? 's' : ''} ago`;
};

// JSON value colors
const JSON_COLORS = {
  null: { color: '#999', fontStyle: 'italic' },
  string: { color: '#0B7500' },
  number: { color: '#1A01CC' },
  boolean: { color: '#1A01CC' },
};

// Common styles
const commonStyles = {
  typography: { fontFamily: 'monospace', variant: 'body2' },
  collapsed: {
    color: '#666',
    cursor: 'pointer',
    '&:hover': { textDecoration: 'underline' },
  },
  expandable: {
    cursor: 'pointer',
    userSelect: 'none',
    '&:hover': { backgroundColor: 'rgba(0,0,0,0.04)' },
    borderRadius: 0.5,
  },
};

export const DevicesHistory = () => {
  const router = useRouter();
  const { httpRequest } = useBackendContext();
  const deviceID = router.query.id?.[0];

  const [messages, setMessages] = useState([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(null);
  const [limit, setLimit] = useState(50);
  const [nextCursor, setNextCursor] = useState('');
  const [hasMore, setHasMore] = useState(false);
  const [autoRefresh, setAutoRefresh] = useState(true);
  const [autoRefreshInterval, setAutoRefreshInterval] = useState(null);
  const [selectedMessage, setSelectedMessage] = useState(null);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [showBasicInfo, setShowBasicInfo] = useState(false);
  const [expandedKeys, setExpandedKeys] = useState(new Set());
  const [showTextView, setShowTextView] = useState(false);
  const bracketRefs = useRef(new Map());

  // Fetch message history
  const fetchMessages = useCallback(async (cursor = '', append = false) => {
    setLoading(true);
    setError(null);
    try {
      const params = new URLSearchParams({ limit: limit.toString() });
      if (cursor) params.append('cursor', cursor);
      const { result, status } = await httpRequest(
        `/api/device/${deviceID}/history?${params.toString()}`,
        'GET'
      );
      if (status === 200 && result) {
        setMessages(append ? prev => [...prev, ...result.messages] : result.messages || []);
        setNextCursor(result.next_cursor || '');
        setHasMore(result.has_more || false);
      } else {
        setError('Failed to fetch message history');
      }
    } catch (err) {
      setError(err.message || 'An error occurred while fetching message history');
    } finally {
      setLoading(false);
    }
  }, [deviceID, limit, httpRequest]);

  useEffect(() => {
    if (deviceID) fetchMessages();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [deviceID, limit]);

  useEffect(() => {
    if (!autoRefresh || !deviceID) {
      if (autoRefreshInterval) {
        clearInterval(autoRefreshInterval);
        setAutoRefreshInterval(null);
      }
      return;
    }
    
    const interval = setInterval(() => fetchMessages(), 5000);
    setAutoRefreshInterval(interval);
    return () => {
      clearInterval(interval);
      setAutoRefreshInterval(null);
    };
  }, [autoRefresh, deviceID, fetchMessages]);

  const handleLoadMore = () => {
    if (nextCursor && !loading) fetchMessages(nextCursor, true);
  };

  const handleRefresh = () => fetchMessages();

  // Recursively collect all paths in a JSON object for full expansion
  const getAllPaths = (obj, prefix = '') => {
    const paths = [];
    if (prefix) paths.push(prefix);
    
    if (typeof obj === 'object' && obj !== null && !Array.isArray(obj)) {
      Object.keys(obj).forEach(key => {
        const path = prefix ? `${prefix}.${key}` : key;
        paths.push(path);
        if (typeof obj[key] === 'object' && obj[key] !== null) {
          paths.push(...getAllPaths(obj[key], path));
        }
      });
    } else if (Array.isArray(obj)) {
      obj.forEach((item, index) => {
        const path = `${prefix}[${index}]`;
        paths.push(path);
        if (typeof item === 'object' && item !== null) {
          paths.push(...getAllPaths(item, path));
        }
      });
    }
    
    return paths;
  };

  const handleRowClick = (message) => {
    setSelectedMessage(message);
    setDialogOpen(true);
    
    // Fully expand all nodes
    const fullRecord = formatMessageForDisplay(message).full_record || {};
    const allPaths = getAllPaths(fullRecord);
    setExpandedKeys(new Set(allPaths));
    
    // Clear bracket refs when opening new message
    bracketRefs.current.clear();
    
    setShowBasicInfo(false);
  };

  // Reorder JSON object keys based on protobuf field order
  const reorderRecordKeys = (obj) => {
    if (!obj || typeof obj !== 'object' || Array.isArray(obj)) return obj;

    const orders = {
      record: ['version', 'to_id', 'from_id', 'payload_security', 'mac_signature', 'sender_cert', 'no_session_context', 'session_context_record'],
      noSessionContext: ['payload'],
      msg: ['header', 'body'],
      header: ['msg_id', 'msg_type'],
      body: ['request', 'response', 'error'],
    };

    const getOrder = (key) => {
      if (key === 'no_session_context') return orders.noSessionContext;
      if (key === 'payload') return orders.msg;
      if (key === 'header') return orders.header;
      if (key === 'body') return orders.body;
      return [];
    };

    const reorderNested = (value, order) => {
      if (!value || typeof value !== 'object' || Array.isArray(value)) return value;
      const nested = {};
      const processed = new Set();
      for (const key of order) {
        if (key in value) {
          nested[key] = reorderNested(value[key], getOrder(key));
          processed.add(key);
        }
      }
      for (const key in value) {
        if (!processed.has(key)) nested[key] = reorderNested(value[key], getOrder(key));
      }
      return nested;
    };

    const reordered = {};
    const processed = new Set();
    for (const key of orders.record) {
      if (key in obj) {
        reordered[key] = reorderNested(obj[key], getOrder(key));
        processed.add(key);
      }
    }
    for (const key in obj) {
      if (!processed.has(key)) reordered[key] = reorderNested(obj[key], getOrder(key));
    }
    return reordered;
  };

  const formatMessageForDisplay = (message) => {
    if (!message) return { error: 'No message data' };
    try {
      const result = {
        id: message.id,
        timestamp: message.timestamp,
        device_serial: message.device_serial,
        direction: message.direction,
        source: message.source === 'device' ? 'Agent' : 'Controller',
        mtp: message.mtp,
        msg_id: message.msg_id,
        msg_type: message.msg_type,
      };
      if (message.full_record) {
        result.full_record = reorderRecordKeys(message.full_record);
      }
      return result;
    } catch (err) {
      return { error: 'Failed to parse message', error_message: err.message, raw: message };
    }
  };

  const toggleExpanded = (path) => {
    setExpandedKeys(prev => {
      const next = new Set(prev);
      next.has(path) ? next.delete(path) : next.add(path);
      return next;
    });
  };

  // Handle bracket click - navigate to matching bracket
  const handleBracketClick = (path, isOpening, bracketType) => {
    const bracketKey = `${path}_${isOpening ? 'open' : 'close'}_${bracketType}`;
    const matchingKey = `${path}_${isOpening ? 'close' : 'open'}_${bracketType}`;
    
    const matchingElement = bracketRefs.current.get(matchingKey);
    if (matchingElement) {
      matchingElement.scrollIntoView({ behavior: 'smooth', block: 'center' });
      // Highlight briefly
      matchingElement.style.backgroundColor = 'rgba(255, 255, 0, 0.3)';
      setTimeout(() => {
        matchingElement.style.backgroundColor = '';
      }, 1000);
    }
  };

  // Render primitive value
  const renderPrimitive = (value, indent) => {
    let content, style;
    if (value === null || value === undefined) {
      content = 'null';
      style = JSON_COLORS.null;
    } else if (typeof value === 'string') {
      content = `"${value}"`;
      style = JSON_COLORS.string;
    } else if (typeof value === 'number' || typeof value === 'boolean') {
      content = String(value);
      style = JSON_COLORS.number;
    } else {
      return null;
    }
    return (
      <Typography component="span" {...commonStyles.typography} sx={{ pl: `${indent}px`, display: 'inline-block' }}>
        <span style={style}>{content}</span>
      </Typography>
    );
  };

  // Render collapsed state
  const renderCollapsed = (type, path, indent) => (
    <Typography
      component="span"
      {...commonStyles.typography}
      sx={{ ...commonStyles.collapsed, pl: `${indent}px` }}
      onClick={(e) => {
        e.stopPropagation();
        toggleExpanded(path);
      }}
    >
      {type === 'object' ? '{...}' : '[...]'}
    </Typography>
  );

  // Render expandable header (chevron + bracket)
  const renderExpandableHeader = (path, isExpanded, indent, isValue, bracket) => {
    const isRoot = path === '';
    const bracketType = bracket === '{' ? 'object' : 'array';
    const bracketKey = `${path}_open_${bracketType}`;
    
    return (
      <Box
        onClick={() => !isRoot && toggleExpanded(path)}
        sx={{
          display: 'inline-flex',
          alignItems: 'center',
          ...(isRoot ? {} : commonStyles.expandable),
          pl: `${indent}px`,
        }}
      >
        {!isRoot && (
          <SvgIcon sx={{ fontSize: 14, mr: 0.5 }}>
            {isExpanded ? <ChevronDownIcon /> : <ChevronRightIcon />}
          </SvgIcon>
        )}
        <Typography
          component="span"
          {...commonStyles.typography}
          ref={(el) => {
            if (el) bracketRefs.current.set(bracketKey, el);
          }}
          onClick={(e) => {
            e.stopPropagation();
            handleBracketClick(path, true, bracketType);
          }}
          sx={{
            cursor: 'pointer',
            '&:hover': { backgroundColor: 'rgba(0, 0, 0, 0.1)' },
            borderRadius: '2px',
            padding: '0 2px',
          }}
        >
          {bracket}
        </Typography>
        {!isExpanded && (
          <>
            {renderCollapsed(isValue ? 'object' : 'array', path, 0)}
            <Typography
              component="span"
              {...commonStyles.typography}
              ref={(el) => {
                if (el) {
                  const closeKey = `${path}_close_${bracketType}`;
                  bracketRefs.current.set(closeKey, el);
                }
              }}
              onClick={(e) => {
                e.stopPropagation();
                handleBracketClick(path, false, bracketType);
              }}
              sx={{
                cursor: 'pointer',
                '&:hover': { backgroundColor: 'rgba(0, 0, 0, 0.1)' },
                borderRadius: '2px',
                padding: '0 2px',
              }}
            >
              {bracket === '{' ? '}' : ']'}
            </Typography>
          </>
        )}
      </Box>
    );
  };

  // Render JSON tree
  const renderJsonTree = (obj, path = '', depth = 0) => {
    const indentSize = 20;
    const indent = depth * indentSize;

    // Primitive values
    const primitive = renderPrimitive(obj, indent);
    if (primitive) return primitive;

    // Arrays
    if (Array.isArray(obj)) {
      const isExpanded = expandedKeys.has(path);
      const isValueArray = path !== '';
      return (
        <Box sx={{ display: 'block' }}>
          {!isValueArray ? (
            renderExpandableHeader(path, isExpanded, indent, false, '[')
          ) : (
            <Box>
              {renderExpandableHeader(path, isExpanded, indent, false, '[')}
            </Box>
          )}
          {isExpanded && (
            <Box>
              {obj.map((item, index) => {
                const itemPath = `${path}[${index}]`;
                const itemIndent = (depth + 1) * indentSize;
                const isExpandable = typeof item === 'object' && item !== null;
                return (
                  <Box key={index} sx={{ mb: 0.5 }}>
                    <Box sx={{ pl: `${itemIndent}px`, display: 'flex', alignItems: 'flex-start' }}>
                      <Typography component="span" {...commonStyles.typography} sx={{ color: '#666', display: 'inline-flex', alignItems: 'baseline' }}>
                        [{index}]:
                        {!isExpandable && (
                          <span style={{ marginLeft: '4px' }}>
                            {renderJsonTree(item, itemPath, 0)}
                          </span>
                        )}
                      </Typography>
                    </Box>
                    {isExpandable && <Box>{renderJsonTree(item, itemPath, depth + 1)}</Box>}
                  </Box>
                );
              })}
              <Box sx={{ pl: `${indent}px` }}>
                <Typography
                  component="span"
                  {...commonStyles.typography}
                  ref={(el) => {
                    if (el) {
                      const closeKey = `${path}_close_array`;
                      bracketRefs.current.set(closeKey, el);
                    }
                  }}
                  onClick={(e) => {
                    e.stopPropagation();
                    handleBracketClick(path, false, 'array');
                  }}
                  sx={{
                    cursor: 'pointer',
                    '&:hover': { backgroundColor: 'rgba(0, 0, 0, 0.1)' },
                    borderRadius: '2px',
                    padding: '0 2px',
                  }}
                >
                  {']'}
                </Typography>
              </Box>
            </Box>
          )}
        </Box>
      );
    }

    // Objects
    if (typeof obj === 'object') {
      const keys = Object.keys(obj);
      const isExpanded = path === '' || expandedKeys.has(path);
      const isValueObject = path !== '';

      if (keys.length === 0) {
        return (
          <Box sx={{ pl: `${indent}px`, display: 'inline-block' }}>
            <span style={{ color: '#999' }}>{'{}'}</span>
          </Box>
        );
      }

      return (
        <Box sx={{ display: 'block' }}>
          {!isValueObject ? (
            renderExpandableHeader(path, isExpanded, indent, true, '{')
          ) : (
            <Box>{renderExpandableHeader(path, isExpanded, indent, true, '{')}</Box>
          )}
          {isExpanded && (
            <Box>
              {keys.map((key) => {
                const keyPath = path ? `${path}.${key}` : key;
                const value = obj[key];
                const isExpandable = typeof value === 'object' && value !== null;
                const isExpandedValue = expandedKeys.has(keyPath);
                const childIndent = (depth + 1) * indentSize;

                return (
                  <Box key={key} sx={{ mb: 0.5 }}>
                    <Box sx={{ pl: `${childIndent}px`, display: 'flex', alignItems: 'flex-start' }}>
                      <Typography component="span" {...commonStyles.typography} sx={{ whiteSpace: 'nowrap', display: 'inline-flex', alignItems: 'baseline' }}>
                        <span style={{ color: '#881391' }}>"{key}"</span>:
                        {!isExpandable && (
                          <span style={{ marginLeft: '4px' }}>
                            {renderJsonTree(value, keyPath, 0)}
                          </span>
                        )}
                      </Typography>
                    </Box>
                    {isExpandable && (
                      <Box>
                        {!isExpandedValue ? (
                          <Box sx={{ pl: `${childIndent + 20}px` }}>
                            {renderCollapsed(Array.isArray(value) ? 'array' : 'object', keyPath, 0)}
                          </Box>
                        ) : (
                          <Box>{renderJsonTree(value, keyPath, depth + 1)}</Box>
                        )}
                      </Box>
                    )}
                  </Box>
                );
              })}
              <Box sx={{ pl: `${indent}px` }}>
                <Typography
                  component="span"
                  {...commonStyles.typography}
                  ref={(el) => {
                    if (el) {
                      const closeKey = `${path}_close_object`;
                      bracketRefs.current.set(closeKey, el);
                    }
                  }}
                  onClick={(e) => {
                    e.stopPropagation();
                    handleBracketClick(path, false, 'object');
                  }}
                  sx={{
                    cursor: 'pointer',
                    '&:hover': { backgroundColor: 'rgba(0, 0, 0, 0.1)' },
                    borderRadius: '2px',
                    padding: '0 2px',
                  }}
                >
                  {'}'}
                </Typography>
              </Box>
            </Box>
          )}
        </Box>
      );
    }

    return (
      <Box sx={{ pl: `${indent}px` }}>
        <span>{String(obj)}</span>
      </Box>
    );
  };

  // Memoize formatted message to avoid repeated processing
  const formattedMessage = useMemo(
    () => selectedMessage ? formatMessageForDisplay(selectedMessage) : null,
    [selectedMessage]
  );
  const jsonString = useMemo(
    () => formattedMessage?.full_record ? JSON.stringify(formattedMessage.full_record, null, 2) : '',
    [formattedMessage?.full_record]
  );

  return (
    <Card>
      <CardHeader title="Message History" subheader={`Messages for device: ${deviceID}`} />
      <CardActions>
        <Stack direction="row" spacing={2} alignItems="center" sx={{ width: '100%', justifyContent: 'space-between' }}>
          <Stack direction="row" spacing={2} alignItems="center">
            <FormControl size="small" sx={{ minWidth: 120 }}>
              <InputLabel>Page Size</InputLabel>
              <Select
                value={limit}
                label="Page Size"
                onChange={(e) => {
                  setLimit(e.target.value);
                  setNextCursor('');
                  setHasMore(false);
                }}
              >
                <MenuItem value={50}>50</MenuItem>
                <MenuItem value={100}>100</MenuItem>
                <MenuItem value={200}>200</MenuItem>
              </Select>
            </FormControl>
            <FormControlLabel
              control={<Switch checked={autoRefresh} onChange={(e) => setAutoRefresh(e.target.checked)} />}
              label="Auto-refresh"
            />
            <IconButton onClick={handleRefresh} disabled={loading}>
              <SvgIcon><ArrowPathIcon /></SvgIcon>
            </IconButton>
          </Stack>
        </Stack>
      </CardActions>
      <CardContent>
        {error && (
          <Box sx={{ mb: 2 }}>
            <Typography color="error">{error}</Typography>
          </Box>
        )}
        <TableContainer component={Paper} sx={{ position: 'relative' }}>
          {loading && messages.length === 0 && (
            <Box sx={{ position: 'absolute', top: 0, left: 0, right: 0, bottom: 0, display: 'flex', alignItems: 'center', justifyContent: 'center', backgroundColor: 'rgba(255, 255, 255, 0.7)', zIndex: 1 }}>
              <CircularProgress />
            </Box>
          )}
          <Table>
            <TableHead>
              <TableRow>
                <TableCell>Timestamp</TableCell>
                <TableCell>Message Type</TableCell>
                <TableCell>Source</TableCell>
                <TableCell>MTP</TableCell>
                <TableCell>Message ID</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {messages.length === 0 && !loading ? (
                <TableRow>
                  <TableCell colSpan={5} align="center">
                    <Typography variant="body2" color="text.secondary">No messages found</Typography>
                  </TableCell>
                </TableRow>
              ) : (
                messages.map((message, index) => {
                  const bgColor = getMessageTypeColor(message.msg_type);
                  // Check if color is light based on luminance (more accurate than hardcoded list)
                  const isLight = ['#66BB6A', '#42A5F5', '#26C6DA', '#AB47BC'].includes(bgColor);
                  return (
                    <TableRow
                      key={message.id || index}
                      onClick={() => handleRowClick(message)}
                      sx={{
                        backgroundColor: `${bgColor}30`,
                        cursor: 'pointer',
                        '&:hover': { backgroundColor: `${bgColor}40` },
                      }}
                    >
                      <TableCell>
                        <Typography variant="body2">{formatTimestamp(message.timestamp)}</Typography>
                        <Typography variant="caption" color="text.secondary">{formatTimeAgo(message.timestamp)}</Typography>
                      </TableCell>
                      <TableCell>
                        <Chip label={message.msg_type} size="small" sx={{ backgroundColor: bgColor, color: isLight ? '#000' : '#fff', fontWeight: 'bold' }} />
                      </TableCell>
                      <TableCell>
                        <Chip
                          label={message.source === 'device' ? 'Agent' : 'Controller'}
                          size="small"
                          sx={{
                            backgroundColor: message.source === 'controller' ? '#1976D2' : '#00ACC1',
                            color: '#fff',
                            fontWeight: 'medium',
                          }}
                        />
                      </TableCell>
                      <TableCell>
                        <Chip label={message.mtp.toUpperCase()} size="small" variant="outlined" />
                      </TableCell>
                      <TableCell>
                        <Typography variant="body2" sx={{ fontFamily: 'monospace', fontSize: '0.75rem' }}>{message.msg_id}</Typography>
                      </TableCell>
                    </TableRow>
                  );
                })
              )}
            </TableBody>
          </Table>
        </TableContainer>
        {hasMore && (
          <Box sx={{ mt: 2, display: 'flex', justifyContent: 'center' }}>
            <Button variant="outlined" onClick={handleLoadMore} disabled={loading}>
              {loading ? <CircularProgress size={20} /> : 'Load More'}
            </Button>
          </Box>
        )}
      </CardContent>

      <Dialog 
        open={dialogOpen} 
        onClose={() => {
          setDialogOpen(false);
          // Cleanup bracket refs when dialog closes
          bracketRefs.current.clear();
        }} 
        maxWidth="md" 
        fullWidth
      >
        <DialogTitle>
          <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
            <Box>
              Message Details
              {selectedMessage && (
                <Typography variant="caption" color="text.secondary" sx={{ ml: 2 }}>
                  {selectedMessage.msg_type} - {selectedMessage.msg_id}
                </Typography>
              )}
            </Box>
            {selectedMessage && (
              <Button
                size="small"
                onClick={() => setShowBasicInfo(!showBasicInfo)}
                startIcon={<SvgIcon sx={{ fontSize: 16 }}>{showBasicInfo ? <ChevronDownIcon /> : <ChevronRightIcon />}</SvgIcon>}
              >
                {showBasicInfo ? 'Hide' : 'Show'} Basic Info
              </Button>
            )}
          </Box>
        </DialogTitle>
        <DialogContent sx={{ overflow: 'hidden', display: 'flex', flexDirection: 'column', height: '70vh' }}>
          {selectedMessage && formattedMessage && (
            <Stack spacing={2} sx={{ height: '100%', overflow: 'hidden', display: 'flex', flexDirection: 'column' }}>
              {showBasicInfo && (
                <Box sx={{ flexShrink: 0 }}>
                  <Typography variant="subtitle2" color="text.secondary" sx={{ mb: 1 }}>Basic Information</Typography>
                  <Divider sx={{ mb: 1 }} />
                  <Stack spacing={1}>
                    <Typography variant="body2"><strong>Timestamp:</strong> {formatTimestamp(selectedMessage.timestamp)}</Typography>
                    <Typography variant="body2"><strong>Device Serial:</strong> {selectedMessage.device_serial}</Typography>
                    <Typography variant="body2"><strong>Source:</strong> {selectedMessage.source === 'device' ? 'Agent' : 'Controller'}</Typography>
                    <Typography variant="body2"><strong>MTP:</strong> {selectedMessage.mtp.toUpperCase()}</Typography>
                    <Typography variant="body2"><strong>Message Type:</strong> {selectedMessage.msg_type}</Typography>
                    <Typography variant="body2"><strong>Message ID:</strong> <span style={{ fontFamily: 'monospace' }}>{selectedMessage.msg_id}</span></Typography>
                  </Stack>
                </Box>
              )}
              <Box sx={{ flex: 1, minHeight: 0, display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
                <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', mb: 1, flexShrink: 0 }}>
                  <Typography variant="subtitle2" color="text.secondary">Full Record Content (JSON)</Typography>
                  <Button
                    size="small"
                    variant="outlined"
                    startIcon={
                      <SvgIcon sx={{ fontSize: 18 }}>
                        {showTextView ? <CodeBracketIcon /> : <DocumentTextIcon />}
                      </SvgIcon>
                    }
                    onClick={() => setShowTextView(!showTextView)}
                  >
                    {showTextView ? 'Tree View' : 'Text View'}
                  </Button>
                </Box>
                <Divider sx={{ mb: 1, flexShrink: 0 }} />
                {showTextView ? (
                  <Box sx={{ flex: 1, minHeight: 0, overflow: 'hidden', display: 'flex', flexDirection: 'column' }}>
                    <TextField
                      multiline
                      fullWidth
                      value={jsonString}
                      InputProps={{
                        readOnly: true,
                        sx: {
                          fontFamily: 'monospace',
                          fontSize: '0.75rem',
                        },
                      }}
                      sx={{
                        flex: 1,
                        minHeight: 0,
                        display: 'flex',
                        '& .MuiInputBase-root': {
                          flex: 1,
                          minHeight: 0,
                          height: '100%',
                          alignItems: 'stretch',
                          backgroundColor: '#f5f5f5',
                        },
                        '& .MuiInputBase-input': {
                          flex: 1,
                          minHeight: 0,
                          height: '100% !important',
                          overflow: 'auto !important',
                          resize: 'none',
                          padding: '16px !important',
                        },
                        '& .MuiOutlinedInput-notchedOutline': {
                          borderColor: 'rgba(0, 0, 0, 0.23)',
                        },
                      }}
                    />
                  </Box>
                ) : (
                  <Box sx={{ backgroundColor: '#f5f5f5', p: 2, borderRadius: 1, flex: 1, overflow: 'auto', fontFamily: 'monospace', fontSize: '0.75rem', minHeight: 0 }}>
                    {renderJsonTree(formattedMessage.full_record || {})}
                  </Box>
                )}
              </Box>
            </Stack>
          )}
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setDialogOpen(false)}>Close</Button>
        </DialogActions>
      </Dialog>
    </Card>
  );
};
