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
  Checkbox,
  Grid,
} from '@mui/material';
import { useRouter } from 'next/router';
import { useBackendContext } from 'src/contexts/backend-context';
import ArrowPathIcon from '@heroicons/react/24/outline/ArrowPathIcon';
import ChevronDownIcon from '@heroicons/react/24/outline/ChevronDownIcon';
import ChevronRightIcon from '@heroicons/react/24/outline/ChevronRightIcon';
import CodeBracketIcon from '@heroicons/react/24/outline/CodeBracketIcon';
import DocumentTextIcon from '@heroicons/react/24/outline/DocumentTextIcon';
import TrashIcon from '@heroicons/react/24/outline/TrashIcon';

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

// Format timestamp to DD/MM/YYYY HH:mm:ss format (24-hour time)
const formatTimestamp = (timestamp) => {
  if (!timestamp) return 'N/A';
  const date = new Date(timestamp);
  const day = String(date.getDate()).padStart(2, '0');
  const month = String(date.getMonth() + 1).padStart(2, '0');
  const year = date.getFullYear();
  const hours = String(date.getHours()).padStart(2, '0');
  const minutes = String(date.getMinutes()).padStart(2, '0');
  const seconds = String(date.getSeconds()).padStart(2, '0');
  return `${day}.${month}.${year} ${hours}:${minutes}:${seconds}`;
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

// Available filter options
// Each item is an array - pairs have 2 elements, singles have 1 element
const MESSAGE_TYPES = [
  ['GET', 'GET_RESP'],
  ['SET', 'SET_RESP'],
  ['ADD', 'ADD_RESP'],
  ['DELETE', 'DELETE_RESP'],
  ['OPERATE', 'OPERATE_RESP'],
  ['NOTIFY', 'NOTIFY_RESP'],
  ['STOMPConnect', 'MQTTConnect'],
  ['Disconnect', 'WebSocketConnect'],
  ['GET_SUPPORTED_DM', 'GET_SUPPORTED_DM_RESP'],
  ['GET_INSTANCES', 'GET_INSTANCES_RESP'],
  ['GET_SUPPORTED_PROTO', 'GET_SUPPORTED_PROTO_RESP'],
  ['REGISTER', 'REGISTER_RESP'],
  ['DEREGISTER', 'DEREGISTER_RESP'],
  ['ERROR'],
  ['SessionContext'],
  ['UNKNOWN_RECORD'],
];

// Flatten for easy lookup (used in getDefaultFilters and Select All)
const MESSAGE_TYPES_FLAT = MESSAGE_TYPES.flat();

const SOURCES = [
  { value: 'controller', label: 'Controller' },
  { value: 'device', label: 'Agent' },
  { value: 'unknown', label: 'Unknown' },
];

const MTPS = [
  { value: 'mqtt', label: 'MQTT' },
  { value: 'ws', label: 'WS' },
  { value: 'stomp', label: 'STOMP' },
  { value: 'unknown', label: 'Unknown' },
];

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
  const newestMessageIdRef = useRef(null); // Track the newest message ID for auto-refresh
  const [selectedMessage, setSelectedMessage] = useState(null);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [showBasicInfo, setShowBasicInfo] = useState(false);
  const [expandedKeys, setExpandedKeys] = useState(new Set());
  const [showTextView, setShowTextView] = useState(false);
  const bracketRefs = useRef(new Map());
  const [fromDate, setFromDate] = useState('');
  const [toDate, setToDate] = useState('');
  const [clearHistoryDialogOpen, setClearHistoryDialogOpen] = useState(false);
  const [clearingHistory, setClearingHistory] = useState(false);
  const [filterDialogOpen, setFilterDialogOpen] = useState(false);
  
  // Get default filter state (all checkboxes selected)
  const getDefaultFilters = () => ({
    messageTypes: [...MESSAGE_TYPES_FLAT], // All message types selected
    sources: SOURCES.map(s => s.value), // All sources selected
    mtps: MTPS.map(m => m.value), // All MTPs selected
    messageId: '',
    messageIdExact: false,
  });
  
  const [filters, setFilters] = useState(getDefaultFilters());
  // Temporary filter state for modal (only applied on "Apply")
  const [tempFilters, setTempFilters] = useState({
    messageTypes: [],
    sources: [],
    mtps: [],
    messageId: '',
    messageIdExact: false,
  });
  const [tempFromDate, setTempFromDate] = useState('');
  const [tempToDate, setTempToDate] = useState('');
  
  // Refs to keep latest filter values for auto-refresh (initialized after state)
  const filtersRef = useRef(filters);
  const fromDateRef = useRef(fromDate);
  const toDateRef = useRef(toDate);
  
  // Keep refs in sync with state
  useEffect(() => {
    filtersRef.current = filters;
    fromDateRef.current = fromDate;
    toDateRef.current = toDate;
  }, [filters, fromDate, toDate]);

  // Update URL query params with current filters (only when user changes filters, not on auto-refresh)
  // Use ref to avoid recreating the function and causing infinite loops
  const updateURLParamsRef = useRef(null);
  updateURLParamsRef.current = () => {
    if (!router.isReady) return;
    
    const currentFilters = filtersRef.current;
    const currentFromDate = fromDateRef.current;
    const currentToDate = toDateRef.current;
    
    // Check if filters are in default state (all selected = no filtering)
    // Need to check both length AND that all expected values are present
    const allMessageTypesSelected = 
      currentFilters.messageTypes.length === MESSAGE_TYPES_FLAT.length &&
      MESSAGE_TYPES_FLAT.every(type => currentFilters.messageTypes.includes(type));
    const allSourcesSelected = 
      currentFilters.sources.length === SOURCES.length &&
      SOURCES.every(s => currentFilters.sources.includes(s.value));
    const allMtpsSelected = 
      currentFilters.mtps.length === MTPS.length &&
      MTPS.every(m => currentFilters.mtps.includes(m.value));
    
    const isDefaultState = 
      allMessageTypesSelected &&
      allSourcesSelected &&
      allMtpsSelected &&
      currentFilters.messageId === '' &&
      currentFromDate === '' &&
      currentToDate === '';
    
    // Build new query - remove filter params if in default state
    const newQuery = { ...router.query };
    
    if (isDefaultState) {
      // Remove all filter params from URL
      delete newQuery.from;
      delete newQuery.to;
      delete newQuery.msg_type;
      delete newQuery.source;
      delete newQuery.mtp;
      delete newQuery.msg_id;
      delete newQuery.msg_id_exact;
    } else {
      // Add/update filter params - handle arrays properly for Next.js router
      if (currentFromDate) newQuery.from = currentFromDate;
      if (currentToDate) newQuery.to = currentToDate;
      
      // Set arrays directly (Next.js router handles arrays in query)
      if (currentFilters.messageTypes.length > 0) {
        newQuery.msg_type = currentFilters.messageTypes;
      } else {
        delete newQuery.msg_type;
      }
      if (currentFilters.sources.length > 0) {
        newQuery.source = currentFilters.sources;
      } else {
        delete newQuery.source;
      }
      if (currentFilters.mtps.length > 0) {
        newQuery.mtp = currentFilters.mtps;
      } else {
        delete newQuery.mtp;
      }
      
      if (currentFilters.messageId) {
        newQuery.msg_id = currentFilters.messageId;
        if (currentFilters.messageIdExact) {
          newQuery.msg_id_exact = 'true';
        } else {
          delete newQuery.msg_id_exact;
        }
      } else {
        delete newQuery.msg_id;
        delete newQuery.msg_id_exact;
      }
    }
    
    // Only update if something actually changed to avoid infinite loops
    const currentQuery = router.query;
    const hasChanges = 
      (currentFromDate !== (currentQuery.from || '')) ||
      (currentToDate !== (currentQuery.to || '')) ||
      (currentFilters.messageId !== (currentQuery.msg_id || '')) ||
      (currentFilters.messageIdExact !== (currentQuery.msg_id_exact === 'true')) ||
      JSON.stringify(currentFilters.messageTypes.sort()) !== JSON.stringify((Array.isArray(currentQuery.msg_type) ? currentQuery.msg_type : currentQuery.msg_type ? [currentQuery.msg_type] : []).sort()) ||
      JSON.stringify(currentFilters.sources.sort()) !== JSON.stringify((Array.isArray(currentQuery.source) ? currentQuery.source : currentQuery.source ? [currentQuery.source] : []).sort()) ||
      JSON.stringify(currentFilters.mtps.sort()) !== JSON.stringify((Array.isArray(currentQuery.mtp) ? currentQuery.mtp : currentQuery.mtp ? [currentQuery.mtp] : []).sort());
    
    if (hasChanges) {
      // Update URL without page reload
      router.push(
        {
          pathname: router.pathname,
          query: newQuery,
        },
        undefined,
        { shallow: true }
      );
    }
  };

  // Read filters from URL query params on mount only (not on every query change)
  const hasReadURLParams = useRef(false);
  useEffect(() => {
    if (!deviceID || !router.isReady || hasReadURLParams.current) return;
    
    const query = router.query;
    
    // If no filter params in URL, keep default state (all selected)
    // Only set filters if filter params are explicitly present in URL
    const hasFilterParams = query.msg_type !== undefined || query.source !== undefined || 
                           query.mtp !== undefined || query.msg_id !== undefined;
    
    if (hasFilterParams) {
      const newFilters = {
        messageTypes: query.msg_type ? (Array.isArray(query.msg_type) ? query.msg_type : [query.msg_type]) : [],
        sources: query.source ? (Array.isArray(query.source) ? query.source : [query.source]) : [],
        mtps: query.mtp ? (Array.isArray(query.mtp) ? query.mtp : [query.mtp]) : [],
        messageId: query.msg_id || '',
        messageIdExact: query.msg_id_exact === 'true',
      };
      setFilters(newFilters);
    }
    // If no filter params, keep default state (already set in useState)
    
    if (query.from) setFromDate(query.from);
    if (query.to) setToDate(query.to);
    hasReadURLParams.current = true;
  }, [deviceID, router.isReady]);
  
  // Reset URL params read flag when device changes
  useEffect(() => {
    hasReadURLParams.current = false;
  }, [deviceID]);

  // Fetch message history
  // mode: 'replace' (default), 'prepend' (for load more - add older messages to top), 'refresh' (for auto-refresh - add new messages to top)
  const fetchMessages = useCallback(async (cursor = '', mode = 'replace') => {
    setLoading(true);
    setError(null);
    try {
      const params = new URLSearchParams({ limit: limit.toString() });
      if (cursor) params.append('cursor', cursor);
      
      // Convert datetime-local (local time) to UTC ISO string for backend
      // datetime-local format: "YYYY-MM-DDTHH:mm" (in user's local timezone)
      if (fromDate) {
        // Create Date object from datetime-local string (interpreted as local time)
        const localDate = new Date(fromDate);
        // Convert to UTC and format as YYYY-MM-DDTHH:mm
        const year = localDate.getUTCFullYear();
        const month = String(localDate.getUTCMonth() + 1).padStart(2, '0');
        const day = String(localDate.getUTCDate()).padStart(2, '0');
        const hours = String(localDate.getUTCHours()).padStart(2, '0');
        const minutes = String(localDate.getUTCMinutes()).padStart(2, '0');
        const utcString = `${year}-${month}-${day}T${hours}:${minutes}`;
        params.append('from', utcString);
      }
      if (toDate) {
        const localDate = new Date(toDate);
        const year = localDate.getUTCFullYear();
        const month = String(localDate.getUTCMonth() + 1).padStart(2, '0');
        const day = String(localDate.getUTCDate()).padStart(2, '0');
        const hours = String(localDate.getUTCHours()).padStart(2, '0');
        const minutes = String(localDate.getUTCMinutes()).padStart(2, '0');
        const utcString = `${year}-${month}-${day}T${hours}:${minutes}`;
        params.append('to', utcString);
      }
      
      // Check if filters are in default state (all selected = no filtering)
      // Need to check both length AND that all expected values are present
      const allMessageTypesSelected = 
        filters.messageTypes.length === MESSAGE_TYPES_FLAT.length &&
        MESSAGE_TYPES_FLAT.every(type => filters.messageTypes.includes(type));
      const allSourcesSelected = 
        filters.sources.length === SOURCES.length &&
        SOURCES.every(s => filters.sources.includes(s.value));
      const allMtpsSelected = 
        filters.mtps.length === MTPS.length &&
        MTPS.every(m => filters.mtps.includes(m.value));
      
      const isDefaultState = 
        allMessageTypesSelected &&
        allSourcesSelected &&
        allMtpsSelected &&
        filters.messageId === '' &&
        !filters.messageIdExact &&
        !fromDate &&
        !toDate;

      // Always send filter params to backend so it knows what filters to apply
      // Message Types
      if (filters.messageTypes.length === 0) {
        // None selected - send empty parameter to indicate "return nothing"
        params.append('msg_type', '');
      } else {
        // Send all selected values
        filters.messageTypes.forEach(type => params.append('msg_type', type));
      }

      // Sources - always send (even in default state, send all values)
      if (filters.sources.length === 0) {
        // None selected - send empty parameter to indicate "return nothing"
        params.append('source', '');
      } else {
        // Send all selected values
        filters.sources.forEach(source => params.append('source', source));
      }

      // MTPs - always send (even in default state, send all values)
      if (filters.mtps.length === 0) {
        // None selected - send empty parameter to indicate "return nothing"
        params.append('mtp', '');
      } else {
        // Send all selected values
        filters.mtps.forEach(mtp => params.append('mtp', mtp));
      }

      // Message ID: only send if set
      if (filters.messageId) {
        params.append('msg_id', filters.messageId);
        if (filters.messageIdExact) {
          params.append('msg_id_exact', 'true');
        }
      }
      
      const { result, status } = await httpRequest(
        `/api/device/${deviceID}/history?${params.toString()}`,
        'GET'
      );
      if (status === 200 && result) {
        const newMessages = result.messages || [];
        
        if (mode === 'replace') {
          // Initial load or manual refresh - replace all messages
          setMessages(newMessages);
          // Track the newest message ID (first in the list since sorted newest first)
          if (newMessages.length > 0) {
            newestMessageIdRef.current = newMessages[0].id;
          }
          setNextCursor(result.next_cursor || '');
          setHasMore(result.has_more || false);
        } else if (mode === 'prepend') {
          // Load More - append older messages to the bottom (they're already sorted newest first)
          setMessages(prev => {
            // Combine existing messages with new messages (older ones)
            // Remove duplicates based on message ID
            const existingIds = new Set(prev.map(m => m.id));
            const uniqueNewMessages = newMessages.filter(m => !existingIds.has(m.id));
            return [...prev, ...uniqueNewMessages];
          });
          setNextCursor(result.next_cursor || '');
          setHasMore(result.has_more || false);
        } else if (mode === 'refresh') {
          // Auto-refresh - only add new messages that are newer than the current newest
          setMessages(prev => {
            if (prev.length === 0) {
              // If list is empty, just set the new messages
              if (newMessages.length > 0) {
                newestMessageIdRef.current = newMessages[0].id;
              }
              return newMessages;
            }
            
            // Find messages that are newer than the current newest
            const currentNewestId = newestMessageIdRef.current;
            if (!currentNewestId) {
              // No reference point, just merge and deduplicate
              const existingIds = new Set(prev.map(m => m.id));
              const uniqueNewMessages = newMessages.filter(m => !existingIds.has(m.id));
              if (uniqueNewMessages.length > 0) {
                newestMessageIdRef.current = uniqueNewMessages[0].id;
              }
              return [...uniqueNewMessages, ...prev];
            }
            
            // Find the index of the newest message in the new results
            const newestIndex = newMessages.findIndex(m => m.id === currentNewestId);
            if (newestIndex === -1) {
              // Current newest not found in new results, all new messages are newer
              const existingIds = new Set(prev.map(m => m.id));
              const uniqueNewMessages = newMessages.filter(m => !existingIds.has(m.id));
              if (uniqueNewMessages.length > 0) {
                newestMessageIdRef.current = uniqueNewMessages[0].id;
              }
              return [...uniqueNewMessages, ...prev];
            }
            
            // Get only messages before the current newest (i.e., newer messages)
            const newerMessages = newMessages.slice(0, newestIndex);
            if (newerMessages.length > 0) {
              // Remove duplicates
              const existingIds = new Set(prev.map(m => m.id));
              const uniqueNewerMessages = newerMessages.filter(m => !existingIds.has(m.id));
              if (uniqueNewerMessages.length > 0) {
                newestMessageIdRef.current = uniqueNewerMessages[0].id;
                return [...uniqueNewerMessages, ...prev];
              }
            }
            
            // No new messages
            return prev;
          });
          // Don't update cursor or hasMore for auto-refresh
        }
      } else {
        setError('Failed to fetch message history');
      }
    } catch (err) {
      setError(err.message || 'An error occurred while fetching message history');
    } finally {
      setLoading(false);
    }
  }, [deviceID, limit, fromDate, toDate, filters, httpRequest]);

  // Track last fetch params to avoid unnecessary refetches
  const lastFetchParamsRef = useRef(null);
  
  useEffect(() => {
    if (!deviceID) return;
    
    // Create a key for current fetch params
    const fetchKey = JSON.stringify({
      deviceID,
      limit,
      fromDate,
      toDate,
      filters,
    });
    
    // Only fetch if params actually changed
    if (lastFetchParamsRef.current === fetchKey) {
      return;
    }
    
    lastFetchParamsRef.current = fetchKey;
    setNextCursor('');
    setHasMore(false);
    newestMessageIdRef.current = null; // Reset when device changes
    fetchMessages('', 'replace');
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [deviceID, limit, fromDate, toDate, filters]);
  
  // Update URL when filters change (but not on initial load or during auto-refresh)
  const isInitialMount = useRef(true);
  const skipNextUpdate = useRef(false);
  const lastUpdateRef = useRef({ fromDate: '', toDate: '', filters: { messageTypes: [], sources: [], mtps: [], messageId: '', messageIdExact: false } });
  
  useEffect(() => {
    if (isInitialMount.current) {
      isInitialMount.current = false;
      lastUpdateRef.current = { fromDate, toDate, filters };
      return;
    }
    if (skipNextUpdate.current) {
      skipNextUpdate.current = false;
      return;
    }
    
    // Only update if values actually changed
    const hasChanged = 
      fromDate !== lastUpdateRef.current.fromDate ||
      toDate !== lastUpdateRef.current.toDate ||
      JSON.stringify(filters) !== JSON.stringify(lastUpdateRef.current.filters);
    
    if (hasChanged && deviceID && router.isReady) {
      lastUpdateRef.current = { fromDate, toDate, filters };
      if (updateURLParamsRef.current) {
        updateURLParamsRef.current();
      }
    }
  }, [deviceID, fromDate, toDate, filters, router.isReady]);

  useEffect(() => {
    if (!autoRefresh || !deviceID) {
      if (autoRefreshInterval) {
        clearInterval(autoRefreshInterval);
        setAutoRefreshInterval(null);
      }
      return;
    }
    
    // Auto-refresh: only fetch new messages and prepend them
    // Use refs to access latest filters/date without recreating the interval
    const interval = setInterval(async () => {
      // Skip URL update during auto-refresh
      skipNextUpdate.current = true;
      
      // Use refs to get latest values without recreating the interval
      const currentFilters = filtersRef.current;
      const currentFromDate = fromDateRef.current;
      const currentToDate = toDateRef.current;
      
      // Build params with current filter values from refs
      const params = new URLSearchParams({ limit: limit.toString() });
      
      if (currentFromDate) {
        const localDate = new Date(currentFromDate);
        const year = localDate.getUTCFullYear();
        const month = String(localDate.getUTCMonth() + 1).padStart(2, '0');
        const day = String(localDate.getUTCDate()).padStart(2, '0');
        const hours = String(localDate.getUTCHours()).padStart(2, '0');
        const minutes = String(localDate.getUTCMinutes()).padStart(2, '0');
        const utcString = `${year}-${month}-${day}T${hours}:${minutes}`;
        params.append('from', utcString);
      }
      if (currentToDate) {
        const localDate = new Date(currentToDate);
        const year = localDate.getUTCFullYear();
        const month = String(localDate.getUTCMonth() + 1).padStart(2, '0');
        const day = String(localDate.getUTCDate()).padStart(2, '0');
        const hours = String(localDate.getUTCHours()).padStart(2, '0');
        const minutes = String(localDate.getUTCMinutes()).padStart(2, '0');
        const utcString = `${year}-${month}-${day}T${hours}:${minutes}`;
        params.append('to', utcString);
      }
      
      // Always send filter params to backend (same logic as fetchMessages)
      // Message Types
      if (currentFilters.messageTypes.length === 0) {
        params.append('msg_type', '');
      } else {
        currentFilters.messageTypes.forEach(type => params.append('msg_type', type));
      }

      // Sources
      if (currentFilters.sources.length === 0) {
        params.append('source', '');
      } else {
        currentFilters.sources.forEach(source => params.append('source', source));
      }

      // MTPs
      if (currentFilters.mtps.length === 0) {
        params.append('mtp', '');
      } else {
        currentFilters.mtps.forEach(mtp => params.append('mtp', mtp));
      }

      // Message ID
      if (currentFilters.messageId) {
        params.append('msg_id', currentFilters.messageId);
        if (currentFilters.messageIdExact) {
          params.append('msg_id_exact', 'true');
        }
      }
      
      // Make the request directly without using fetchMessages to avoid dependency issues
      try {
        const { result, status } = await httpRequest(
          `/api/device/${deviceID}/history?${params.toString()}`,
          'GET'
        );
        if (status === 200 && result) {
          const newMessages = result.messages || [];
          setMessages(prev => {
            if (prev.length === 0) {
              if (newMessages.length > 0) {
                newestMessageIdRef.current = newMessages[0].id;
              }
              return newMessages;
            }
            
            const currentNewestId = newestMessageIdRef.current;
            if (!currentNewestId) {
              const existingIds = new Set(prev.map(m => m.id));
              const uniqueNewMessages = newMessages.filter(m => !existingIds.has(m.id));
              if (uniqueNewMessages.length > 0) {
                newestMessageIdRef.current = uniqueNewMessages[0].id;
              }
              return [...uniqueNewMessages, ...prev];
            }
            
            const newestIndex = newMessages.findIndex(m => m.id === currentNewestId);
            if (newestIndex === -1) {
              const existingIds = new Set(prev.map(m => m.id));
              const uniqueNewMessages = newMessages.filter(m => !existingIds.has(m.id));
              if (uniqueNewMessages.length > 0) {
                newestMessageIdRef.current = uniqueNewMessages[0].id;
              }
              return [...uniqueNewMessages, ...prev];
            }
            
            const newerMessages = newMessages.slice(0, newestIndex);
            if (newerMessages.length > 0) {
              const existingIds = new Set(prev.map(m => m.id));
              const uniqueNewerMessages = newerMessages.filter(m => !existingIds.has(m.id));
              if (uniqueNewerMessages.length > 0) {
                newestMessageIdRef.current = uniqueNewerMessages[0].id;
                return [...uniqueNewerMessages, ...prev];
              }
            }
            
            return prev;
          });
        }
      } catch (err) {
        // Silently fail during auto-refresh to avoid spamming errors
        console.error('Auto-refresh error:', err);
      }
    }, 5000);
    setAutoRefreshInterval(interval);
    return () => {
      clearInterval(interval);
      setAutoRefreshInterval(null);
    };
    // Only recreate interval when autoRefresh, deviceID, or limit changes
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [autoRefresh, deviceID, limit, httpRequest]);

  const handleLoadMore = () => {
    if (nextCursor && !loading) fetchMessages(nextCursor, 'prepend');
  };

  const handleRefresh = () => fetchMessages();

  // Open filter modal and copy current filters to temp state, or use defaults if no filters applied
  const handleOpenFilterDialog = () => {
    // If no filters are applied, use default state (all selected)
    const hasFilters = filters.messageTypes.length > 0 || 
                       filters.sources.length > 0 || 
                       filters.mtps.length > 0 || 
                       filters.messageId !== '' ||
                       fromDate !== '' ||
                       toDate !== '';
    
    if (hasFilters) {
      setTempFilters({ ...filters });
      setTempFromDate(fromDate);
      setTempToDate(toDate);
    } else {
      setTempFilters(getDefaultFilters());
      setTempFromDate('');
      setTempToDate('');
    }
    setFilterDialogOpen(true);
  };
  
  // Apply filters from temp state
  const handleApplyFilters = () => {
    // Check if filters are in default state (all selected = no filtering)
    // Need to check both length AND that all expected values are present
    const allMessageTypesSelected = 
      tempFilters.messageTypes.length === MESSAGE_TYPES_FLAT.length &&
      MESSAGE_TYPES_FLAT.every(type => tempFilters.messageTypes.includes(type));
    const allSourcesSelected = 
      tempFilters.sources.length === SOURCES.length &&
      SOURCES.every(s => tempFilters.sources.includes(s.value));
    const allMtpsSelected = 
      tempFilters.mtps.length === MTPS.length &&
      MTPS.every(m => tempFilters.mtps.includes(m.value));
    
    const isDefaultState = 
      allMessageTypesSelected &&
      allSourcesSelected &&
      allMtpsSelected &&
      tempFilters.messageId === '' &&
      tempFromDate === '' &&
      tempToDate === '';
    
    if (isDefaultState) {
      // Set filters to default state (all selected)
      setFilters(getDefaultFilters());
      setFromDate('');
      setToDate('');
    } else {
      // Apply the filters
      setFilters({ ...tempFilters });
      setFromDate(tempFromDate);
      setToDate(tempToDate);
    }
    
    setNextCursor('');
    setHasMore(false);
    setFilterDialogOpen(false);
  };
  
  // Cancel - just close modal without applying
  const handleCancelFilters = () => {
    setFilterDialogOpen(false);
  };
  
  // Clear all filters - return to default state (all selected)
  const handleClearFilters = () => {
    setTempFilters(getDefaultFilters());
    setTempFromDate('');
    setTempToDate('');
  };
  
  // Handle temp filter changes in modal
  const handleTempFilterChange = (filterType, value, checked) => {
    setTempFilters(prev => {
      const newFilters = { ...prev };
      if (filterType === 'messageType') {
        if (checked) {
          newFilters.messageTypes = [...prev.messageTypes, value];
        } else {
          newFilters.messageTypes = prev.messageTypes.filter(t => t !== value);
        }
      } else if (filterType === 'source') {
        if (checked) {
          newFilters.sources = [...prev.sources, value];
        } else {
          newFilters.sources = prev.sources.filter(s => s !== value);
        }
      } else if (filterType === 'mtp') {
        if (checked) {
          newFilters.mtps = [...prev.mtps, value];
        } else {
          newFilters.mtps = prev.mtps.filter(m => m !== value);
        }
      }
      return newFilters;
    });
  };
  
  const handleTempMessageIdChange = (value) => {
    setTempFilters(prev => ({ ...prev, messageId: value }));
  };
  
  const handleTempMessageIdExactChange = (checked) => {
    setTempFilters(prev => ({ ...prev, messageIdExact: checked }));
  };

  const handleClearHistory = async () => {
    setClearingHistory(true);
    try {
      const { status } = await httpRequest(
        `/api/device/${deviceID}/history`,
        'DELETE'
      );
      if (status === 200) {
        // Clear the messages list and reset pagination
        setMessages([]);
        setNextCursor('');
        setHasMore(false);
        setClearHistoryDialogOpen(false);
        // Optionally show success message
      } else {
        setError('Failed to clear message history');
      }
    } catch (err) {
      setError(err.message || 'An error occurred while clearing message history');
    } finally {
      setClearingHistory(false);
    }
  };

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
        <Stack direction="row" spacing={2} alignItems="center" sx={{ width: '100%', justifyContent: 'space-between', flexWrap: 'wrap' }}>
          <Stack direction="row" spacing={2} alignItems="center" sx={{ flexWrap: 'wrap' }}>
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
            <Button
              variant="outlined"
              onClick={handleOpenFilterDialog}
              disabled={loading}
            >
              Filters
            </Button>
            <Button
              variant="outlined"
              color="error"
              startIcon={<SvgIcon><TrashIcon /></SvgIcon>}
              onClick={() => setClearHistoryDialogOpen(true)}
              disabled={loading}
              sx={{
                '&:hover': {
                  backgroundColor: 'error.main',
                },
              }}
            >
              Clear History
            </Button>
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

      {/* Clear History Confirmation Dialog */}
      <Dialog
        open={clearHistoryDialogOpen}
        onClose={() => !clearingHistory && setClearHistoryDialogOpen(false)}
      >
        <DialogTitle>Clear Message History</DialogTitle>
        <DialogContent>
          <Typography>
            Are you sure you want to clear history and permanently delete all of the messages for device <strong>{deviceID}</strong>?
          </Typography>
        </DialogContent>
        <DialogActions>
          <Button
            onClick={() => setClearHistoryDialogOpen(false)}
            disabled={clearingHistory}
          >
            Cancel
          </Button>
          <Button
            onClick={handleClearHistory}
            color="error"
            variant="contained"
            disabled={clearingHistory}
            startIcon={clearingHistory ? <CircularProgress size={16} /> : <SvgIcon><TrashIcon /></SvgIcon>}
          >
            {clearingHistory ? 'Clearing...' : 'Yes'}
          </Button>
        </DialogActions>
      </Dialog>

      {/* Filter Dialog */}
      <Dialog 
        open={filterDialogOpen} 
        onClose={handleCancelFilters}
        maxWidth={false}
        PaperProps={{
          sx: { maxWidth: 500 }
        }}
      >
        <DialogTitle>Filters</DialogTitle>
        <DialogContent>
          <Stack spacing={3} sx={{ mt: 1 }}>
            {/* Date Range */}
            <Box>
              <Typography variant="subtitle2" sx={{ mb: 1 }}>Date Range</Typography>
              <Stack direction="row" spacing={2}>
                <TextField
                  label="From"
                  type="datetime-local"
                  value={tempFromDate}
                  onChange={(e) => setTempFromDate(e.target.value)}
                  InputLabelProps={{ shrink: true }}
                  size="small"
                  fullWidth
                />
                <TextField
                  label="To"
                  type="datetime-local"
                  value={tempToDate}
                  onChange={(e) => setTempToDate(e.target.value)}
                  InputLabelProps={{ shrink: true }}
                  size="small"
                  fullWidth
                />
              </Stack>
            </Box>

            {/* Message Type - Pairs layout */}
            <Box>
              <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 1 }}>
                <Typography variant="subtitle2">Message Type</Typography>
                <Stack direction="row" spacing={1}>
                  <Button
                    size="small"
                    variant="outlined"
                    onClick={() => {
                      setTempFilters(prev => ({
                        ...prev,
                        messageTypes: [...MESSAGE_TYPES_FLAT],
                      }));
                    }}
                  >
                    Select All
                  </Button>
                  <Button
                    size="small"
                    variant="outlined"
                    onClick={() => {
                      setTempFilters(prev => ({
                        ...prev,
                        messageTypes: [],
                      }));
                    }}
                  >
                    Unselect All
                  </Button>
                </Stack>
              </Box>
              <Box sx={{ border: '1px solid #e0e0e0', borderRadius: 1, p: 1.5 }}>
                <Grid container spacing={1}>
                  {MESSAGE_TYPES.map((types, index) => (
                    <Grid item xs={12} key={index}>
                      {types.length === 2 ? (
                        // Pair - display side by side
                        <Stack direction="row" spacing={2}>
                          <FormControlLabel
                            control={
                              <Checkbox
                                checked={tempFilters.messageTypes.includes(types[0])}
                                onChange={(e) => handleTempFilterChange('messageType', types[0], e.target.checked)}
                                size="small"
                                sx={{
                                  '& .MuiSvgIcon-root': {
                                    fontSize: '18px',
                                  },
                                }}
                              />
                            }
                            label={types[0]}
                            sx={{ 
                              flex: 1, 
                              m: 0, 
                              fontSize: '0.75rem',
                              border: '1px solid #e0e0e0',
                              borderRadius: '4px',
                              padding: '4px 8px',
                              marginRight: '8px',
                            }}
                          />
                          <FormControlLabel
                            control={
                              <Checkbox
                                checked={tempFilters.messageTypes.includes(types[1])}
                                onChange={(e) => handleTempFilterChange('messageType', types[1], e.target.checked)}
                                size="small"
                                sx={{
                                  '& .MuiSvgIcon-root': {
                                    fontSize: '18px',
                                  },
                                }}
                              />
                            }
                            label={types[1]}
                            sx={{ 
                              flex: 1, 
                              m: 0, 
                              fontSize: '0.75rem',
                              border: '1px solid #e0e0e0',
                              borderRadius: '4px',
                              padding: '4px 8px',
                              marginRight: '8px',
                            }}
                          />
                        </Stack>
                      ) : (
                        // Single - display alone
                        <FormControlLabel
                          control={
                            <Checkbox
                              checked={tempFilters.messageTypes.includes(types[0])}
                              onChange={(e) => handleTempFilterChange('messageType', types[0], e.target.checked)}
                              size="small"
                              sx={{
                                '& .MuiSvgIcon-root': {
                                  fontSize: '18px',
                                },
                              }}
                            />
                          }
                          label={types[0]}
                          sx={{ 
                            m: 0, 
                            fontSize: '0.75rem',
                            border: '1px solid #e0e0e0',
                            borderRadius: '4px',
                            padding: '4px 8px',
                            marginRight: '8px',
                          }}
                        />
                      )}
                    </Grid>
                  ))}
                </Grid>
              </Box>
            </Box>

            {/* Source and MTP on one line */}
            <Box>
              <Grid container spacing={2}>
                <Grid item xs={6}>
                  <Typography variant="subtitle2" sx={{ mb: 1 }}>Source</Typography>
                  <Box sx={{ border: '1px solid #e0e0e0', borderRadius: 1, p: 1.5 }}>
                    <Stack spacing={1}>
                      {SOURCES.map((source) => (
                        <FormControlLabel
                          key={source.value}
                          control={
                            <Checkbox
                              checked={tempFilters.sources.includes(source.value)}
                              onChange={(e) => handleTempFilterChange('source', source.value, e.target.checked)}
                              size="small"
                              sx={{
                                '& .MuiSvgIcon-root': {
                                  fontSize: '18px',
                                },
                              }}
                            />
                          }
                          label={source.label}
                          sx={{ 
                            m: 0,
                            fontSize: '0.75rem',
                            border: '1px solid #e0e0e0',
                            borderRadius: '4px',
                            padding: '4px 8px',
                          }}
                        />
                      ))}
                    </Stack>
                  </Box>
                  <Stack direction="row" spacing={1} sx={{ mt: 1, justifyContent: 'center' }}>
                    <Button
                      size="small"
                      variant="outlined"
                      onClick={() => {
                        setTempFilters(prev => ({
                          ...prev,
                          sources: SOURCES.map(s => s.value),
                        }));
                      }}
                    >
                      All
                    </Button>
                    <Button
                      size="small"
                      variant="outlined"
                      onClick={() => {
                        setTempFilters(prev => ({
                          ...prev,
                          sources: [],
                        }));
                      }}
                    >
                      None
                    </Button>
                  </Stack>
                </Grid>
                <Grid item xs={6}>
                  <Typography variant="subtitle2" sx={{ mb: 1 }}>MTP</Typography>
                  <Box sx={{ border: '1px solid #e0e0e0', borderRadius: 1, p: 1.5 }}>
                    <Stack spacing={1}>
                      {MTPS.map((mtp) => (
                        <FormControlLabel
                          key={mtp.value}
                          control={
                            <Checkbox
                              checked={tempFilters.mtps.includes(mtp.value)}
                              onChange={(e) => handleTempFilterChange('mtp', mtp.value, e.target.checked)}
                              size="small"
                              sx={{
                                '& .MuiSvgIcon-root': {
                                  fontSize: '18px',
                                },
                              }}
                            />
                          }
                          label={mtp.label}
                          sx={{ 
                            m: 0,
                            fontSize: '0.75rem',
                            border: '1px solid #e0e0e0',
                            borderRadius: '4px',
                            padding: '4px 8px',
                          }}
                        />
                      ))}
                    </Stack>
                  </Box>
                  <Stack direction="row" spacing={1} sx={{ mt: 1, justifyContent: 'center' }}>
                    <Button
                      size="small"
                      variant="outlined"
                      onClick={() => {
                        setTempFilters(prev => ({
                          ...prev,
                          mtps: MTPS.map(m => m.value),
                        }));
                      }}
                    >
                      All
                    </Button>
                    <Button
                      size="small"
                      variant="outlined"
                      onClick={() => {
                        setTempFilters(prev => ({
                          ...prev,
                          mtps: [],
                        }));
                      }}
                    >
                      None
                    </Button>
                  </Stack>
                </Grid>
              </Grid>
            </Box>

            {/* Message ID */}
            <Box>
              <Typography variant="subtitle2" sx={{ mb: 1 }}>Message ID</Typography>
              <Stack direction="row" spacing={2} alignItems="center" sx={{ flexWrap: 'nowrap' }}>
                <TextField
                  label="Message ID"
                  value={tempFilters.messageId}
                  onChange={(e) => handleTempMessageIdChange(e.target.value)}
                  size="small"
                  sx={{ flex: 1, minWidth: 0 }}
                />
                <FormControlLabel
                  control={
                    <Checkbox
                      checked={tempFilters.messageIdExact}
                      onChange={(e) => handleTempMessageIdExactChange(e.target.checked)}
                      size="small"
                      sx={{
                        '& .MuiSvgIcon-root': {
                          fontSize: '18px',
                        },
                      }}
                    />
                  }
                  label="Exact match"
                  sx={{ 
                    fontSize: '0.75rem',
                    whiteSpace: 'nowrap',
                    flexShrink: 0,
                    border: '1px solid #e0e0e0',
                    borderRadius: '4px',
                    padding: '4px 8px',
                    marginRight: '8px',
                  }}
                />
              </Stack>
            </Box>
          </Stack>
        </DialogContent>
        <DialogActions>
          <Button onClick={handleClearFilters}>Clear</Button>
          <Button onClick={handleCancelFilters}>Cancel</Button>
          <Button onClick={handleApplyFilters} variant="contained">Apply</Button>
        </DialogActions>
      </Dialog>
    </Card>
  );
};
