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
  Dialog,
  DialogActions,
  DialogContent,
  DialogContentText,
  DialogTitle,
  TextField,
  Button,
  Backdrop,
  Alert,
  Typography,
  Popover,
  Checkbox,
  FormControlLabel,
  Grid,
  Tabs,
  Tab,
  Paper,
  Chip,
  InputAdornment,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Stack,
  OutlinedInput,
} from '@mui/material';
import CircularProgress from '@mui/material/CircularProgress';
import ArrowPathIcon from '@heroicons/react/24/outline/ArrowPathIcon';
import PlusCircleIcon from '@heroicons/react/24/outline/PlusCircleIcon';
import Pencil from "@heroicons/react/24/outline/PencilIcon";
import XMarkIcon from '@heroicons/react/24/outline/XMarkIcon';
import TrashIcon from '@heroicons/react/24/outline/TrashIcon';
import PlayCircleIcon from '@heroicons/react/24/outline/PlayCircleIcon';
import CheckIcon from '@heroicons/react/24/outline/CheckIcon';
import MagnifyingGlassIcon from '@heroicons/react/24/outline/MagnifyingGlassIcon';
import DocumentTextIcon from '@heroicons/react/24/outline/DocumentTextIcon';
import CommandLineIcon from '@heroicons/react/24/outline/CommandLineIcon';
import { useRouter } from 'next/router';
import { useTenant } from 'src/contexts/tenant-context';
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

const ObjAccessType = {
  ReadOnly: 0,
  AddDelete: 1,
  AddOnly: 2,
  DeleteOnly: 3,
};

const ParamAccessType = {
  ReadOnly: 0,
  ReadWrite: 1,
  WriteOnly: 2,
};

const pathToUrl = (tr181Path) => tr181Path.replace(/\.$/, '').replaceAll('.', '/');
const segmentsToPath = (segments) => segments.length > 0 ? segments.join('.') + '.' : 'Device.';

const sortInstanceKeys = (a, b) => {
  const numsA = a.match(/\d+/g)?.map(Number) || [];
  const numsB = b.match(/\d+/g)?.map(Number) || [];
  for (let i = 0; i < Math.min(numsA.length, numsB.length); i++) {
    if (numsA[i] !== numsB[i]) return numsA[i] - numsB[i];
  }
  return numsA.length - numsB.length;
};

const getObjName = (path) => {
  const parts = path.replace(/\.$/, '').split('.');
  for (let i = parts.length - 1; i >= 0; i--) {
    if (parts[i] !== '{i}') return parts[i];
  }
  return parts[parts.length - 1];
};

const getRelativeChildPath = (childPath, parentPath) => {
  if (childPath.startsWith(parentPath)) {
    return childPath.substring(parentPath.length);
  }
  return childPath;
};

const isDirectChild = (childPath, parentPath) => {
  const rel = getRelativeChildPath(childPath, parentPath);
  if (!rel) return false;
  const parts = rel.replace(/\.$/, '').split('.');
  const nameSegments = parts.filter(p => p !== '{i}');
  return nameSegments.length === 1;
};

const getChildDisplayName = (relativePath) => {
  const parts = relativePath.replace(/\.$/, '').split('.');
  return parts.find(p => p !== '{i}') || parts[0];
};

const isMultiInstance = (relativePath) => relativePath.includes('{i}');

const buildInstancePattern = (templatePath) =>
  new RegExp('^' + templatePath.replace(/\{i\}/g, '\\d+').replace(/\./g, '\\.') + '$');

const isMultiInstanceTemplate = (supportedObjPath) => {
  if (!supportedObjPath) return false;
  const parts = supportedObjPath.replace(/\.$/, '').split('.');
  return parts[parts.length - 1] === '{i}';
};

const getMultiInstanceTablePath = (path) => {
  if (!path?.includes('{i}')) return path;
  // Remove the instance template segment: "....Foo.{i}." -> "....Foo."
  const withoutTemplate = path.replace(/\.\{i\}(\.$|$)/, (_, suffix) => suffix || '.');
  return withoutTemplate.replace(/\.{2,}/g, '.');
};

const normalizeNavigationPath = (path) => getMultiInstanceTablePath(path);

const resolveTreeChildPath = (childPath) => getMultiInstanceTablePath(childPath);

const getInstanceLabel = (instancePath) => {
  const parts = instancePath.replace(/\.$/, '').split('.');
  for (let i = parts.length - 1; i >= 0; i--) {
    if (/^\d+$/.test(parts[i])) return parts[i];
  }
  return getObjName(instancePath);
};

const getInstanceKeys = (values, templatePath) => {
  if (!templatePath || !values) return [];
  const pattern = buildInstancePattern(templatePath);
  return Object.keys(values)
    .filter(k => pattern.test(k) && Array.isArray(values[k]))
    .sort(sortInstanceKeys);
};

const isMultiInstanceView = (mainObj, values) =>
  isMultiInstanceTemplate(mainObj?.supported_obj_path) &&
  getInstanceKeys(values, mainObj.supported_obj_path).length > 0;

const isConcreteInstancePath = (path, templatePath) =>
  isMultiInstanceTemplate(templatePath) && buildInstancePattern(templatePath).test(path);

const getConcreteInstanceTablePath = (path) =>
  /\.\d+\.$/.test(path) ? path.replace(/\.\d+\.$/, '.') : null;

const mergeChildPaths = (paths) => {
  const unique = [...new Set(
    paths.filter(Boolean).filter(p => !p.includes('..') && !p.includes('{i}'))
  )];
  return unique.sort((a, b) => {
    if (/\.\d+\.$/.test(a) && /\.\d+\.$/.test(b)) return sortInstanceKeys(a, b);
    return a.localeCompare(b);
  });
};

const buildSchemaChildPaths = (supportedObjs, mainObj, parentPath) =>
  mergeChildPaths(
    supportedObjs
      .slice(1)
      .filter(child => isDirectChild(child.supported_obj_path, mainObj.supported_obj_path))
      .map(child => resolveTreeChildPath(child.supported_obj_path))
      .filter(childPath => childPath !== parentPath)
  );

const buildGetValues = (supportedParams, templatePath, fetchPath, getResult) => {
  const values = {};
  if (!supportedParams?.length || !getResult?.req_path_results) return values;

  const templateParts = templatePath.split('.');
  const inputParts = fetchPath.split('.');
  const concreteObjPath = templateParts.map((seg, idx) => {
    if (seg === '{i}') {
      if (idx < inputParts.length && /^\d+$/.test(inputParts[idx])) return inputParts[idx];
      return '*';
    }
    return seg;
  }).join('.');

  const paramsInfo = {};
  supportedParams.forEach(p => {
    paramsInfo[p.param_name] = {
      value_change: p.value_change,
      value_type: p.value_type,
      access: p.access,
      value: '-',
    };
  });

  getResult.req_path_results.forEach(x => {
    if (!x.resolved_path_results) return;

    const parts = x.requested_path.split('.');
    if (parts[parts.length - 2] === '*') {
      x.resolved_path_results.forEach(y => {
        if (!y.result_params) return;
        const key = Object.keys(y.result_params)[0];
        if (!key) return;
        if (!values[y.resolved_path]) values[y.resolved_path] = [];
        const val = y.result_params[key] === '' ? '""' : y.result_params[key];
        values[y.resolved_path].push({
          [key]: { ...paramsInfo[key], value: val },
        });
      });
    } else {
      const rpr = x.resolved_path_results[0];
      if (!rpr?.result_params) return;
      Object.keys(rpr.result_params).forEach(key => {
        const val = rpr.result_params[key];
        values[key] = {
          ...paramsInfo[key],
          value: val === '' ? '""' : val,
        };
      });
    }
  });

  return values;
};

const upsertInstanceTreeNodes = (treeNodes, values, templatePath, tablePath) => {
  const childPaths = getInstanceKeys(values, templatePath);
  let next = { ...treeNodes };

  childPaths.forEach(instanceKey => {
    next[instanceKey] = {
      ...(next[instanceKey] || {
        path: instanceKey,
        expanded: false,
        loaded: false,
        loading: false,
        children: [],
      }),
      path: instanceKey,
      name: getInstanceLabel(instanceKey),
      isMultiInstance: true,
    };
  });

  next[tablePath] = {
    ...(next[tablePath] || { path: tablePath, name: getObjName(tablePath) }),
    path: tablePath,
    expanded: true,
    instancesLoaded: true,
    children: mergeChildPaths(childPaths),
  };

  return linkNodeToAncestors(next, tablePath);
};

const linkNodeToAncestors = (treeNodes, targetPath) => {
  const parts = targetPath.replace(/\.$/, '').split('.');
  let next = { ...treeNodes };

  for (let i = 1; i <= parts.length; i++) {
    const seg = parts[i - 1];
    if (seg === '{i}') continue;

    const anc = parts.slice(0, i).join('.') + '.';
    const isInstance = /^\d+$/.test(seg);

    if (!next[anc]) {
      next[anc] = {
        path: anc,
        name: isInstance ? seg : seg,
        expanded: !isInstance,
        loaded: false,
        loading: false,
        children: [],
        isMultiInstance: isInstance,
      };
    }

    if (i > 1) {
      const parentAnc = parts.slice(0, i - 1).join('.') + '.';
      if (next[parentAnc]) {
        next[parentAnc] = {
          ...next[parentAnc],
          expanded: true,
          children: mergeChildPaths([...(next[parentAnc].children || []), anc]),
        };
      }
    }
  }

  return next;
};

const extractCommandName = (commandPath) => {
  if (!commandPath) return '';
  const parts = commandPath.split('.');
  return parts[parts.length - 1].replace(/\(\)$/, '');
};

const generateUniqueCommandKey = (commandPath) => {
  const commandName = extractCommandName(commandPath);
  if (!commandName) return '';
  const now = new Date();
  const pad = (n) => String(n).padStart(2, '0');
  const randomHex = Math.floor(Math.random() * 0x10000).toString(16).padStart(4, '0');
  return `${commandName}_${now.getFullYear()}${pad(now.getMonth()+1)}${pad(now.getDate())}_${pad(now.getHours())}${pad(now.getMinutes())}${pad(now.getSeconds())}_${randomHex}`;
};

const getAuthHeaders = () => {
  const headers = new Headers();
  headers.append("Content-Type", "application/json");
  headers.append("Authorization", localStorage.getItem("token"));
  return headers;
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
  const { apiPrefix } = useTenant();

  // Derive device ID and current TR-181 path from URL
  const deviceID = router.query.id?.[0];
  const pathSegments = useMemo(() => router.query.id?.slice(2) || [], [router.query.id]);
  const pathKey = pathSegments.join('/');
  const currentPath = useMemo(() => segmentsToPath(pathSegments), [pathSegments]);

  // Tab State
  const [activeTab, setActiveTab] = useState(0);

  // Tree and Schema state
  const [treeNodes, setTreeNodes] = useState({
    'Device.': {
      path: 'Device.',
      name: 'Device',
      expanded: true,
      loaded: false,
      loading: false,
      children: [],
      isMultiInstance: false,
    }
  });

  // Inline parameter editing
  const [editingParam, setEditingParam] = useState(null); // Full path parameter name being inline edited
  const [editingValue, setEditingValue] = useState('');

  // Live in-place Tree Search
  const [treeSearchQuery, setTreeSearchQuery] = useState('');
  const searchRunRef = useRef(0);

  // Original state holders for Right Details Panel
  const [deviceParameters, setDeviceParameters] = useState(null);
  const [deviceParametersValue, setDeviceParametersValue] = useState({});
  const [showLoading, setShowLoading] = useState(false);
  const [errorModal, setErrorModal] = useState(false);
  const [errorModalText, setErrorModalText] = useState("");
  const [errorModalTitle, setErrorModalTitle] = useState("Response");
  const [deviceOfflineError, setDeviceOfflineError] = useState(false);
  const [deviceOfflineErrorText, setDeviceOfflineErrorText] = useState("");
  const [openCommandDialog, setOpenCommandDialog] = useState(false);
  const [deviceCommandToExecute, setDeviceCommandToExecute] = useState(null);
  const [inputArgsValue, setInputArgsValue] = useState({});
  const [addDialog, setAddDialog] = useState(null);
  const [addParamValues, setAddParamValues] = useState({});
  const [addParamRequired, setAddParamRequired] = useState({});
  const [addAllowPartial, setAddAllowPartial] = useState(true);
  const [addResult, setAddResult] = useState(null);

  // Navigate to a TR-181 path by updating Next.js URL
  const navigateTo = useCallback((tr181Path) => {
    const normalizedPath = normalizeNavigationPath(tr181Path);
    const urlPath = pathToUrl(normalizedPath);
    router.push(
      `/devices/usp/${deviceID}/discovery/${urlPath}`,
      undefined,
      { shallow: true }
    );
  }, [deviceID, router]);

  // Navigate up one level
  const navigateBack = useCallback(() => {
    const parts = currentPath.replace(/\.$/, '').split('.');
    if (parts.length <= 1) return;

    parts.pop();
    if (parts.length > 1 && /^\d+$/.test(parts[parts.length - 1])) {
      parts.pop();
    }

    const parentPath = parts.join('.') + '.';
    navigateTo(parentPath);
  }, [currentPath, navigateTo]);

  // API helpers
  const fetchWithAuth = useCallback(async (endpoint, body) => {
    const result = await fetch(
      `${process.env.NEXT_PUBLIC_REST_ENDPOINT || ""}${apiPrefix}/device/${deviceID}/any/${endpoint}`,
      { method: 'PUT', headers: getAuthHeaders(), redirect: 'follow', body: JSON.stringify(body) }
    );
    if (result.status === 401) {
      router.push("/auth/login");
      return null;
    }
    if (result.status !== 200) {
      let errorText = await result.text();
      try {
        const errorJson = JSON.parse(errorText);
        errorText = typeof errorJson === 'string' ? errorJson : JSON.stringify(errorJson, null, 2);
      } catch (e) {}
      errorText = errorText.trim().replace(/^["']|["']$/g, '');
      throw new Error(errorText || `Request failed with status ${result.status}`);
    }
    return result.json();
  }, [deviceID, router, apiPrefix]);

  // Recursive ancestor populator for smooth tree expansion on deep load or search jump
  const ensurePathInTree = useCallback((targetPath) => {
    const normalizedPath = normalizeNavigationPath(targetPath);
    const parts = normalizedPath.replace(/\.$/, '').split('.');
    const ancestors = [];
    for (let i = 1; i <= parts.length; i++) {
      if (parts[i - 1] === '{i}') continue;
      ancestors.push(parts.slice(0, i).join('.') + '.');
    }

    setTreeNodes(prev => {
      let updated = { ...prev };
      ancestors.forEach((anc, idx) => {
        const seg = parts[idx] || anc;
        if (seg === '{i}') return;
        const isInstance = /^\d+$/.test(seg);

        if (!updated[anc]) {
          updated[anc] = {
            path: anc,
            name: seg,
            expanded: !isInstance,
            loaded: false,
            loading: false,
            children: [],
            isMultiInstance: isInstance,
          };
        } else {
          updated[anc] = {
            ...updated[anc],
            expanded: isInstance ? updated[anc].expanded : true,
          };
        }

        if (idx > 0) {
          const parentAnc = ancestors[idx - 1];
          if (updated[parentAnc]) {
            updated[parentAnc] = {
              ...updated[parentAnc],
              children: mergeChildPaths([...(updated[parentAnc].children || []), anc]),
            };
          }
        }
      });
      return updated;
    });
  }, []);

  const loadTableInstanceChildren = useCallback(async (tablePath) => {
    let alreadyLoaded = false;
    setTreeNodes(prev => {
      alreadyLoaded = !!prev[tablePath]?.instancesLoaded;
      return prev;
    });
    if (alreadyLoaded) return;

    try {
      const content = await fetchWithAuth('parameters', {
        obj_paths: [tablePath],
        first_level_only: true,
        return_commands: false,
        return_events: false,
        return_params: true,
      });

      const mainObj = content?.req_obj_results?.[0]?.supported_objs?.[0];
      const supportedParams = mainObj?.supported_params;
      const templatePath = mainObj?.supported_obj_path;
      if (!mainObj || !supportedParams?.length || !isMultiInstanceTemplate(templatePath)) return;

      const templateParts = templatePath.split('.');
      const inputParts = tablePath.split('.');
      const concreteObjPath = templateParts.map((seg, idx) => {
        if (seg === '{i}') {
          if (idx < inputParts.length && /^\d+$/.test(inputParts[idx])) return inputParts[idx];
          return '*';
        }
        return seg;
      }).join('.');
      const paramsToFetch = supportedParams.map(p => concreteObjPath + p.param_name);

      const result = await fetchWithAuth('get', {
        param_paths: paramsToFetch,
        max_depth: 1,
      });

      const values = buildGetValues(supportedParams, templatePath, tablePath, result);
      setTreeNodes(prev => {
        if (prev[tablePath]?.instancesLoaded) return prev;
        return upsertInstanceTreeNodes(prev, values, templatePath, tablePath);
      });
    } catch {
      // Tree enrichment is best-effort; the details panel already loaded.
    }
  }, [fetchWithAuth]);

  // Main data fetching function
  const updateDeviceParameters = useCallback(async (path, { preserveState = false } = {}) => {
    const fetchPath = normalizeNavigationPath(path);
    onStatusRefresh?.();
    setShowLoading(true);
    if (!preserveState) {
      setDeviceParameters(null);
      setDeviceParametersValue({});
    }

    try {
      const content = await fetchWithAuth('parameters', {
        obj_paths: [fetchPath],
        first_level_only: true,
        return_commands: true,
        return_events: true,
        return_params: true,
      });

      if (!content?.req_obj_results?.[0]?.supported_objs?.length) {
        setErrorModalText("Invalid response structure from device.");
        setErrorModal(true);
        return;
      }

      const supportedObjs = content.req_obj_results[0].supported_objs;
      const mainObj = supportedObjs[0];
      const children = supportedObjs.slice(1).filter(
        child => isDirectChild(child.supported_obj_path, mainObj.supported_obj_path)
      );

      children.sort((a, b) => getObjName(a.supported_obj_path).localeCompare(getObjName(b.supported_obj_path)));

      const supportedParams = mainObj.supported_params;
      let values = {};

      if (supportedParams?.length) {
        const paramsToFetch = supportedParams.map(p => {
          const templateParts = mainObj.supported_obj_path.split('.');
          const inputParts = fetchPath.split('.');
          const concreteObjPath = templateParts.map((seg, idx) => {
            if (seg === '{i}') {
              if (idx < inputParts.length && /^\d+$/.test(inputParts[idx])) return inputParts[idx];
              return '*';
            }
            return seg;
          }).join('.');
          return concreteObjPath + p.param_name;
        });

        const result = await fetchWithAuth('get', {
          param_paths: paramsToFetch,
          max_depth: 1,
        });

        values = buildGetValues(supportedParams, mainObj.supported_obj_path, fetchPath, result);
        setDeviceParametersValue(values);
      }

      setDeviceParameters(content);

      const templatePath = mainObj.supported_obj_path;
      const tablePath = isMultiInstanceTemplate(templatePath)
        ? getMultiInstanceTablePath(templatePath)
        : null;
      const onConcreteInstance = tablePath && isConcreteInstancePath(fetchPath, templatePath);
      const treeUpdatePath = onConcreteInstance ? fetchPath : (tablePath || fetchPath);
      const showAsInstance = isMultiInstanceView(mainObj, values);

      setTreeNodes(prev => {
        let childPaths = [];
        let newTreeNodes = { ...prev };

        const upsertSchemaChildren = (parentPath) => {
          const paths = buildSchemaChildPaths(supportedObjs, mainObj, parentPath);
          paths.forEach(childPath => {
            const schemaChild = children.find(
              c => resolveTreeChildPath(c.supported_obj_path) === childPath
            );
            const relPath = schemaChild
              ? getRelativeChildPath(schemaChild.supported_obj_path, templatePath)
              : getRelativeChildPath(childPath, parentPath);

            if (!newTreeNodes[childPath]) {
              newTreeNodes[childPath] = {
                path: childPath,
                name: getObjName(childPath),
                expanded: false,
                loaded: false,
                loading: false,
                children: [],
                isMultiInstance: isMultiInstance(relPath),
              };
            }
          });
          return paths;
        };

        if (showAsInstance && tablePath) {
          newTreeNodes = upsertInstanceTreeNodes(newTreeNodes, values, templatePath, tablePath);
          newTreeNodes[tablePath] = {
            ...newTreeNodes[tablePath],
            loaded: true,
            loading: false,
          };
        } else if (onConcreteInstance && tablePath) {
          childPaths = upsertSchemaChildren(fetchPath);
          newTreeNodes[fetchPath] = {
            ...(newTreeNodes[fetchPath] || {
              path: fetchPath,
              name: getInstanceLabel(fetchPath),
              isMultiInstance: true,
            }),
            path: fetchPath,
            name: getInstanceLabel(fetchPath),
            isMultiInstance: true,
            loaded: true,
            loading: false,
            children: childPaths,
          };

          if (newTreeNodes[tablePath]) {
            newTreeNodes[tablePath] = {
              ...newTreeNodes[tablePath],
              expanded: true,
            };
          }
        } else {
          childPaths = upsertSchemaChildren(treeUpdatePath);
          newTreeNodes[treeUpdatePath] = {
            ...(newTreeNodes[treeUpdatePath] || { path: treeUpdatePath, name: getObjName(treeUpdatePath) }),
            loaded: true,
            loading: false,
            children: childPaths,
          };
        }

        return newTreeNodes;
      });

      if (onConcreteInstance && tablePath) {
        await loadTableInstanceChildren(tablePath);
      }

    } catch (error) {
      const errorMsg = error.message || "An error occurred while retrieving device parameters.";
      if (errorMsg.toLowerCase().includes("offline") || errorMsg.includes("503")) {
        setDeviceOfflineErrorText(errorMsg);
        setDeviceOfflineError(true);
      } else {
        setErrorModalText(errorMsg);
        setErrorModal(true);
      }
    } finally {
      setShowLoading(false);
    }
  }, [fetchWithAuth, loadTableInstanceChildren, onStatusRefresh]);

  // Toggle tree node expanded state
  const handleToggleNode = useCallback((path) => {
    setTreeNodes(prev => {
      const node = prev[path];
      if (!node) return prev;

      const nextExpanded = !node.expanded;
      if (nextExpanded && !node.loaded) {
        setTimeout(() => updateDeviceParameters(normalizeNavigationPath(path)), 0);
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
  }, [updateDeviceParameters]);

  const loadTreePathForSearch = useCallback(async (path) => {
    if (!deviceID || !path) return [];

    try {
      const content = await fetchWithAuth('parameters', {
        obj_paths: [path],
        first_level_only: true,
        return_commands: false,
        return_events: false,
        return_params: false,
      });

      const supportedObjs = content?.req_obj_results?.[0]?.supported_objs || [];
      if (supportedObjs.length === 0) return [];

      const mainObj = supportedObjs[0];
      const treePath = normalizeNavigationPath(path);
      let childPaths = buildSchemaChildPaths(supportedObjs, mainObj, treePath);

      setTreeNodes(prev => {
        const next = { ...prev };

        if (isMultiInstanceTemplate(mainObj.supported_obj_path)) {
          const tablePath = getMultiInstanceTablePath(mainObj.supported_obj_path);
          const existingChildren = next[tablePath]?.children || [];
          const instancePattern = buildInstancePattern(mainObj.supported_obj_path);
          const instanceChildren = existingChildren.filter(k => instancePattern.test(k));
          if (instanceChildren.length > 0) {
            childPaths = mergeChildPaths([...childPaths, ...instanceChildren]);
          }
        }

        childPaths.forEach(childPath => {
          const schemaChild = supportedObjs
            .slice(1)
            .find(c => resolveTreeChildPath(c.supported_obj_path) === childPath);
          const relPath = schemaChild
            ? getRelativeChildPath(schemaChild.supported_obj_path, mainObj.supported_obj_path)
            : getRelativeChildPath(childPath, treePath);

          if (!next[childPath]) {
            next[childPath] = {
              path: childPath,
              name: getObjName(childPath),
              expanded: false,
              loaded: false,
              loading: false,
              children: [],
              isMultiInstance: isMultiInstance(relPath),
            };
          }
        });

        const updatePath = isMultiInstanceTemplate(mainObj.supported_obj_path)
          ? getMultiInstanceTablePath(mainObj.supported_obj_path)
          : treePath;

        next[updatePath] = {
          ...(next[updatePath] || { path: updatePath, name: getObjName(updatePath) }),
          path: updatePath,
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
  }, [deviceID, fetchWithAuth]);

  useEffect(() => {
    const query = treeSearchQuery.trim();
    if (query.length < 2) return undefined;

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
      crawl(['Device.']);
    }, 300);

    return () => {
      clearTimeout(timer);
      searchRunRef.current += 1;
    };
  }, [treeSearchQuery, loadTreePathForSearch]);

  // Redirect legacy/template URLs and sync tree + details panel
  useEffect(() => {
    if (!deviceID || !currentPath) return;

    const normalizedPath = normalizeNavigationPath(currentPath);
    if (normalizedPath !== currentPath) {
      router.replace(
        `/devices/usp/${deviceID}/discovery/${pathToUrl(normalizedPath)}`,
        undefined,
        { shallow: true }
      );
      return;
    }

    ensurePathInTree(normalizedPath);
    updateDeviceParameters(normalizedPath);
  }, [currentPath, deviceID, ensurePathInTree, updateDeviceParameters, router]);

  // CRUD API functions preserved intact
  const openAddDialog = async (objPath, childTemplatePath) => {
    setAddDialog({ objPath, params: null });
    setAddParamValues({});
    setAddParamRequired({});
    setAddAllowPartial(true);

    try {
      const content = await fetchWithAuth('parameters', {
        obj_paths: [childTemplatePath],
        first_level_only: true,
        return_commands: false,
        return_events: false,
        return_params: true,
      });

      const supportedParams = content?.req_obj_results?.[0]?.supported_objs?.[0]?.supported_params || [];
      const writableParams = supportedParams.filter(
        p => p.access === ParamAccessType.ReadWrite || p.access === ParamAccessType.WriteOnly
      );
      setAddDialog({ objPath, params: writableParams });
    } catch (error) {
      setAddDialog({ objPath, params: [] });
    }
  };

  const closeAddDialog = () => {
    setAddDialog(null);
    setAddParamValues({});
    setAddParamRequired({});
  };

  const submitAddInstance = async () => {
    if (!addDialog) return;
    const { objPath } = addDialog;

    const paramSettings = Object.entries(addParamValues)
      .filter(([, value]) => value !== '' && value != null)
      .map(([param, value]) => ({
        param,
        value,
        required: !!addParamRequired[param],
      }));

    closeAddDialog();
    setShowLoading(true);
    try {
      const createObj = { obj_path: objPath };
      if (paramSettings.length > 0) {
        createObj.param_settings = paramSettings;
      }
      const result = await fetchWithAuth('add', {
        allow_partial: addAllowPartial,
        create_objs: [createObj],
      });
      if (result) {
        const objResult = result.created_obj_results?.[0];
        const operStatus = objResult?.oper_status?.OperStatus;
        const instantiatedPath = operStatus?.OperSuccess?.instantiated_path;
        const failedParams = new Set();

        if (operStatus?.OperSuccess) {
          const paramErrs = operStatus.OperSuccess.param_errs || [];
          paramErrs.forEach(pe => failedParams.add(pe.param));
          const isPartial = paramErrs.length > 0;
          setAddResult({
            status: 'success',
            message: instantiatedPath
              ? `${isPartial ? 'Partially successfully' : 'Successfully'} created ${instantiatedPath}:`
              : `${isPartial ? 'Partially successfully' : 'Successfully'} created:`,
            result,
            failedParams,
          });
        } else if (operStatus?.OperFailure) {
          const paramErrs = operStatus.OperFailure.param_errs || [];
          paramErrs.forEach(pe => failedParams.add(pe.param));
          setAddResult({
            status: 'failure',
            message: `Couldn't create new instance for ${objPath}:`,
            result,
            failedParams,
          });
        } else {
          setAddResult({
            status: 'error',
            message: `Couldn't create new instance for ${objPath}:`,
            result,
            failedParams,
          });
        }
        updateDeviceParameters(currentPath);
      }
    } catch (error) {
      setAddResult({
        status: 'error',
        message: `Couldn't create new instance for ${objPath}:`,
        result: { error: error.message },
        failedParams: new Set(),
      });
    } finally {
      setShowLoading(false);
    }
  };

  const deleteDeviceObj = async (objPath) => {
    setShowLoading(true);
    try {
      const result = await fetchWithAuth('del', {
        allow_partial: true,
        obj_paths: [objPath],
      });
      if (result) {
        updateDeviceParameters(currentPath);
      }
    } catch (error) {
      setErrorModalText(error.message);
      setErrorModal(true);
    } finally {
      setShowLoading(false);
    }
  };

  // Inline Parameter editing setter
  const applyInlineEdit = async (fullParamPath, paramValue) => {
    const params = fullParamPath.split('.');
    const parameterToChange = params.pop();
    const objToChange = params.join('.') + '.';

    setEditingParam(null);
    setShowLoading(true);

    try {
      const result = await fetchWithAuth('set', {
        allow_partial: true,
        update_objs: [{
          obj_path: objToChange,
          param_settings: [{
            param: parameterToChange,
            value: paramValue,
            required: true,
          }],
        }],
      });

      if (!result) return;

      const feedback = JSON.stringify(result, null, 2);
      if (!result.updated_obj_results?.[0]?.oper_status?.OperStatus?.OperSuccess) {
        setErrorModalText(feedback);
        setErrorModal(true);
        return;
      }

      if (/^\d+$/.test(params[params.length - 1])) {
        setDeviceParametersValue(prev => ({
          ...prev,
          [objToChange]: prev[objToChange]?.map(el => {
            if (el[parameterToChange] !== undefined) {
              return { ...el, [parameterToChange]: { ...el[parameterToChange], value: paramValue } };
            }
            return el;
          }),
        }));
      } else {
        setDeviceParametersValue(prev => ({
          ...prev,
          [parameterToChange]: { ...prev[parameterToChange], value: paramValue },
        }));
      }
    } catch (error) {
      setErrorModalText(error.message);
      setErrorModal(true);
    } finally {
      setShowLoading(false);
    }
  };

  const applyCommand = async () => {
    const commandPath = Object.keys(deviceCommandToExecute)[0];
    const commandKey = generateUniqueCommandKey(commandPath);

    setShowLoading(true);
    try {
      const result = await fetchWithAuth('operate', {
        command: commandPath,
        command_key: commandKey,
        input_args: inputArgsValue || {},
        send_resp: true,
      });

      setInputArgsValue({});
      setDeviceCommandToExecute(null);
      setOpenCommandDialog(false);

      if (result) {
        setErrorModalText(JSON.stringify(result, null, 2));
        setErrorModal(true);
      }
    } catch (error) {
      setErrorModalText(error.message);
      setErrorModal(true);
      setInputArgsValue({});
      setDeviceCommandToExecute(null);
      setOpenCommandDialog(false);
    } finally {
      setShowLoading(false);
    }
  };

  // Tree Filtering recursive helper for local Search In-Place
  const shouldShowNode = useCallback((path, query) => {
    if (!query) return true;
    const q = query.toLowerCase();
    if (path.toLowerCase().includes(q)) return true;

    const node = treeNodes[path];
    if (!node || !node.children) return false;
    return node.children.some(childPath => shouldShowNode(childPath, query));
  }, [treeNodes]);

  // Hierarchical list renderer for Lefthand tree nodes with auto filter and auto expand
  const renderTreeList = (nodePaths) => {
    if (!nodePaths || nodePaths.length === 0) return null;

    // Filter paths based on search query
    const filteredPaths = nodePaths.filter(path => shouldShowNode(path, treeSearchQuery));

    // Sorting: objects alphabetically, multi-instances numerically
    const sorted = [...filteredPaths].sort((a, b) => {
      const nodeA = treeNodes[a];
      const nodeB = treeNodes[b];
      if (nodeA?.isMultiInstance && nodeB?.isMultiInstance) {
        return sortInstanceKeys(a, b);
      }
      return a.localeCompare(b);
    });

    return (
      <List dense disablePadding>
        {sorted.map(path => {
          const node = treeNodes[path];
          if (!node) return null;

          const isSelected = normalizeNavigationPath(currentPath) === path;
          const hasChildren = node.children && node.children.length > 0;
          const isExpandable = hasChildren || (!node.loaded && path.endsWith('.') && !node.isMultiInstance);
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
                  primary={node.name + (node.isMultiInstance ? '' : '.')}
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

  // Right pane header breadcrumbs renderer
  const renderBreadcrumbs = () => {
    const segments = currentPath.replace(/\.$/, '').split('.').filter(seg => seg !== '{i}');
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

  // Sort parameter list: normal parameters alphabetically, NumberOfEntries counts last
  const sortParams = (params, getName = (p) => p) => {
    const list = Array.isArray(params) ? params : [];
    return list.sort((a, b) => {
      const nameA = getName(a);
      const nameB = getName(b);
      const aIsCount = nameA.endsWith('NumberOfEntries');
      const bIsCount = nameB.endsWith('NumberOfEntries');
      if (aIsCount !== bIsCount) return aIsCount ? 1 : -1;
      return nameA.localeCompare(nameB);
    });
  };

  const mainObjForView = deviceParameters?.req_obj_results?.[0]?.supported_objs?.[0];

  // Find if current node has numerical values to plot
  const hasNumericalParams = useMemo(() => {
    let count = 0;
    if (!isMultiInstanceView(mainObjForView, deviceParametersValue)) {
      Object.entries(deviceParametersValue).forEach(([name, data]) => {
        const type = String(data.value_type || '').toLowerCase();
        const isNumeric = ['int', 'unsignedint', 'long', 'float', 'double', 'dateTime'].some(t => type.includes(t)) ||
          ['packets', 'bytes', 'errors', 'signal', 'noise', 'rate', 'time', 'temperature', 'power', 'utilization'].some(k => name.toLowerCase().includes(k));
        if (isNumeric && data.value !== '-' && !isNaN(Number(data.value))) {
          count++;
        }
      });
    }
    return count > 0;
  }, [deviceParametersValue, mainObjForView]);

  // Loading animation overlay
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
            {/* Tree search bar / entry */}
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

            {/* Collapsible tree scrolling panel */}
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
              {renderTreeList(['Device.'])}
            </Box>
          </Grid>

          {/* RIGHT COLUMN: Tabbed Active Node details */}
          <Grid item xs={12} md={7.5} sx={{ display: 'flex', flexDirection: 'column', bgcolor: 'background.paper' }}>

            {/* Header with Breadcrumbs & Add/Delete Instance actions */}
            {deviceParameters?.req_obj_results?.[0] ? (
              <Box sx={{ p: 2, borderBottom: '1px solid', borderColor: 'divider', display: 'flex', alignItems: 'center', justifyContent: 'space-between', flexWrap: 'wrap', gap: 1 }}>
                {renderBreadcrumbs()}

                {/* Dynamic buttons for multi-instance tables */}
                {(() => {
                  const mainObj = deviceParameters.req_obj_results[0].supported_objs[0];
                  const access = mainObj.access;
                  const canAdd = access === ObjAccessType.AddDelete || access === ObjAccessType.AddOnly;
                  const canDelete = access === ObjAccessType.AddDelete || access === ObjAccessType.DeleteOnly;
                  const onConcreteInstance = isConcreteInstancePath(currentPath, mainObj.supported_obj_path);
                  const addPath = isMultiInstanceTemplate(mainObj.supported_obj_path)
                    ? getMultiInstanceTablePath(mainObj.supported_obj_path)
                    : currentPath;

                  return (
                    <Box sx={{ display: 'flex', gap: 1 }}>
                      {canAdd && !onConcreteInstance && (
                        <Button
                          variant="contained"
                          size="small"
                          startIcon={<SvgIcon fontSize="small"><PlusCircleIcon /></SvgIcon>}
                          onClick={() => openAddDialog(addPath, mainObj.supported_obj_path)}
                        >
                          Add Instance
                        </Button>
                      )}
                      {canDelete && onConcreteInstance && (
                        <Button
                          variant="outlined"
                          color="error"
                          size="small"
                          startIcon={<SvgIcon fontSize="small"><TrashIcon /></SvgIcon>}
                          onClick={() => deleteDeviceObj(currentPath)}
                        >
                          Delete Instance
                        </Button>
                      )}
                    </Box>
                  );
                })()}
              </Box>
            ) : null}

            {/* Selection tabs */}
            <Tabs
              value={activeTab}
              onChange={(e, val) => setActiveTab(val)}
              borderbottom={1}
              sx={{
                px: 2,
                borderBottom: '1px solid',
                borderColor: 'divider',
                minHeight: '48px',
                '& .MuiTab-root': { py: 1.5, minHeight: '48px', fontWeight: 600 }
              }}
            >
              <Tab label="Parameters" icon={<SvgIcon fontSize="small"><DocumentTextIcon /></SvgIcon>} iconPosition="start" />
              <Tab label="Commands" icon={<SvgIcon fontSize="small"><CommandLineIcon /></SvgIcon>} iconPosition="start" />
            </Tabs>

            {/* Details dynamic display */}
            <Box sx={{ p: 3, flexGrow: 1, maxHeight: '748px', overflowY: 'auto' }}>
              {deviceOfflineError && deviceOfflineErrorText && (
                <Alert
                  severity="error"
                  onClose={() => { setDeviceOfflineError(false); setDeviceOfflineErrorText(""); }}
                  sx={{ mb: 2 }}
                >
                  {deviceOfflineErrorText}
                </Alert>
              )}

              {!deviceParameters ? renderLoading() : (
                <>
                  {/* TAB 1: Parameters Property Table */}
                  {activeTab === 0 && (() => {
                    const mainObj = deviceParameters.req_obj_results[0].supported_objs[0];
                    const showInstanceTable = isMultiInstanceView(mainObj, deviceParametersValue);

                    if (showInstanceTable) {
                      const instanceKeys = getInstanceKeys(deviceParametersValue, mainObj.supported_obj_path);
                      return (
                        <Stack spacing={3}>
                          {instanceKeys.map(instanceKey => {
                            const params = deviceParametersValue[instanceKey] || [];
                            const sortedParams = sortParams(params, p => Object.keys(p)[0]);

                            return (
                              <Card key={instanceKey} variant="outlined" sx={{ border: '1px solid', borderColor: 'divider' }}>
                                <Box sx={{ py: 1, px: 2, display: 'flex', justifyContent: 'space-between', alignItems: 'center', bgcolor: 'background.neutral', borderBottom: '1px solid', borderColor: 'divider' }}>
                                  <Typography variant="subtitle2" sx={{ fontWeight: 700 }}>
                                    {getInstanceLabel(instanceKey)}
                                  </Typography>
                                  {mainObj.access === ObjAccessType.AddDelete && (
                                    <IconButton size="small" color="error" onClick={() => deleteDeviceObj(instanceKey)}>
                                      <SvgIcon fontSize="small"><TrashIcon /></SvgIcon>
                                    </IconButton>
                                  )}
                                </Box>
                                <TableContainer>
                                  <Table size="small">
                                    <TableHead>
                                      <TableRow>
                                        <TableCell sx={{ fontWeight: 600 }}>Parameter Name</TableCell>
                                        <TableCell sx={{ fontWeight: 600 }}>Data Type</TableCell>
                                        <TableCell sx={{ fontWeight: 600, textAlign: 'right', pr: 4 }}>Value</TableCell>
                                      </TableRow>
                                    </TableHead>
                                    <TableBody>
                                      {sortedParams.map(param => {
                                        const paramName = Object.keys(param)[0];
                                        const paramData = param[paramName];
                                        const isWritable = paramData.access > ParamAccessType.ReadOnly;

                                        const fullParamPath = instanceKey + paramName;
                                        const isEditing = editingParam === fullParamPath;

                                        return (
                                          <TableRow key={paramName} hover>
                                            <TableCell sx={{ py: 1 }}>{paramName}</TableCell>
                                            <TableCell>
                                              <Chip label={paramData.value_type || 'string'} size="small" variant="outlined" sx={{ height: '20px', fontSize: '10px' }} />
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
                                                      if (e.key === 'Enter') applyInlineEdit(fullParamPath, editingValue);
                                                      if (e.key === 'Escape') setEditingParam(null);
                                                    }}
                                                    sx={{
                                                      width: '200px',
                                                      '& .MuiInputBase-input': { py: 0.5, fontSize: '13px' }
                                                    }}
                                                  />
                                                  <IconButton size="small" color="primary" onClick={() => applyInlineEdit(fullParamPath, editingValue)}>
                                                    <SvgIcon fontSize="small"><CheckIcon /></SvgIcon>
                                                  </IconButton>
                                                  <IconButton size="small" onClick={() => setEditingParam(null)}>
                                                    <SvgIcon fontSize="small"><XMarkIcon /></SvgIcon>
                                                  </IconButton>
                                                </Box>
                                              ) : (
                                                <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'flex-end', gap: 1 }}>
                                                  <ValueDisplay value={paramData.value} />
                                                  {isWritable && (
                                                    <IconButton
                                                      size="small"
                                                      onClick={() => {
                                                        setEditingParam(fullParamPath);
                                                        setEditingValue(paramData.value ?? '');
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
                              </Card>
                            );
                          })}
                        </Stack>
                      );
                    }

                    // Standard Flat Parameter List
                    if (!mainObj.supported_params?.length) {
                      return (
                        <Box sx={{ py: 6, textAlign: 'center' }}>
                          <Typography variant="body2" color="text.secondary">No parameters found under this node.</Typography>
                        </Box>
                      );
                    }

                    const sortedParams = sortParams(mainObj.supported_params, p => p.param_name);
                    return (
                      <TableContainer component={Paper} variant="outlined" sx={{ border: '1px solid', borderColor: 'divider' }}>
                        <Table size="small">
                          <TableHead sx={{ bgcolor: 'background.neutral' }}>
                            <TableRow>
                              <TableCell sx={{ fontWeight: 700 }}>Parameter Name</TableCell>
                              <TableCell sx={{ fontWeight: 700 }}>Data Type</TableCell>
                              <TableCell sx={{ fontWeight: 700 }}>Writable</TableCell>
                              <TableCell sx={{ fontWeight: 700, textAlign: 'right', pr: 4 }}>Value</TableCell>
                            </TableRow>
                          </TableHead>
                          <TableBody>
                            {sortedParams.map(p => {
                              const paramData = deviceParametersValue[p.param_name];
                              const isWritable = paramData?.access > ParamAccessType.ReadOnly;
                              const paramValue = paramData?.value;

                              const fullParamPath = currentPath + p.param_name;
                              const isEditing = editingParam === fullParamPath;

                              return (
                                <TableRow key={p.param_name} hover>
                                  <TableCell sx={{ fontWeight: 500, py: 1 }}>{p.param_name}</TableCell>
                                  <TableCell>
                                    <Chip label={p.value_type} size="small" variant="outlined" sx={{ height: '20px', fontSize: '10px' }} />
                                  </TableCell>
                                  <TableCell>
                                    {isWritable ? (
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
                                            if (e.key === 'Enter') applyInlineEdit(fullParamPath, editingValue);
                                            if (e.key === 'Escape') setEditingParam(null);
                                          }}
                                          sx={{
                                            width: '200px',
                                            '& .MuiInputBase-input': { py: 0.5, fontSize: '13px' }
                                          }}
                                        />
                                        <IconButton size="small" color="primary" onClick={() => applyInlineEdit(fullParamPath, editingValue)}>
                                          <SvgIcon fontSize="small"><CheckIcon /></SvgIcon>
                                        </IconButton>
                                        <IconButton size="small" onClick={() => setEditingParam(null)}>
                                          <SvgIcon fontSize="small"><XMarkIcon /></SvgIcon>
                                        </IconButton>
                                      </Box>
                                    ) : (
                                      <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'flex-end', gap: 1 }}>
                                        <ValueDisplay value={paramValue} />
                                        {isWritable && (
                                          <IconButton
                                            size="small"
                                            onClick={() => {
                                              setEditingParam(fullParamPath);
                                              setEditingValue(paramValue ?? '');
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

                  {/* TAB 2: Commands Methods Execution List */}
                  {activeTab === 1 && (() => {
                    const mainObj = deviceParameters.req_obj_results[0].supported_objs[0];
                    if (!mainObj.supported_commands?.length) {
                      return (
                        <Box sx={{ py: 8, textAlign: 'center' }}>
                          <Typography variant="body2" color="text.secondary">No executable commands available for this node.</Typography>
                        </Box>
                      );
                    }

                    const sortedCommands = [...mainObj.supported_commands].sort((a, b) => a.command_name.localeCompare(b.command_name));
                    return (
                      <Stack spacing={2}>
                        {sortedCommands.map(cmd => {
                          const fullCmdPath = currentPath + cmd.command_name;
                          return (
                            <Card key={cmd.command_name} variant="outlined" sx={{ p: 2, display: 'flex', justifyContent: 'space-between', alignItems: 'center', border: '1px solid', borderColor: 'divider' }}>
                              <Box>
                                <Typography variant="subtitle2" sx={{ fontWeight: 700, color: 'primary.main' }}>
                                  {cmd.command_name}()
                                </Typography>
                                {cmd.input_arg_names && cmd.input_arg_names.length > 0 && (
                                  <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mt: 0.5 }}>
                                    Arguments: {cmd.input_arg_names.join(', ')}
                                  </Typography>
                                )}
                              </Box>
                              <Button
                                variant="contained"
                                size="small"
                                color="primary"
                                startIcon={<SvgIcon fontSize="small"><PlayCircleIcon /></SvgIcon>}
                                onClick={() => {
                                  setDeviceCommandToExecute({
                                    [fullCmdPath]: { input_arg_names: cmd.input_arg_names }
                                  });
                                  setOpenCommandDialog(true);
                                }}
                              >
                                Execute
                              </Button>
                            </Card>
                          );
                        })}
                      </Stack>
                    );
                  })()}
                </>
              )}
            </Box>
          </Grid>

        </Grid>
      </CardContent>

      {/* ORIGINAL MODALS PRESERVED ABSOLUTELY UNTOUCHED */}
      {addDialog && (
        <Dialog
          open
          onClose={closeAddDialog}
          slotProps={{ backdrop: { sx: { bgcolor: (theme) => theme.palette.mode === 'dark' ? 'rgba(0,0,0,0.5)' : 'rgba(255,255,255,0.5)' } } }}
          fullWidth
          maxWidth="sm"
        >
          <DialogTitle>Add Instance: {addDialog.objPath}</DialogTitle>
          <DialogContent>
            <FormControlLabel
              control={
                <Checkbox
                  checked={addAllowPartial}
                  onChange={(e) => setAddAllowPartial(e.target.checked)}
                />
              }
              label="Allow partial"
              sx={{ mb: 2 }}
            />
            {addDialog.params === null ? (
              <Box sx={{ display: 'flex', justifyContent: 'center', py: 2 }}>
                <CircularProgress size={24} />
              </Box>
            ) : addDialog.params.length > 0 ? (
              [...addDialog.params]
                .sort((a, b) => a.param_name.localeCompare(b.param_name))
                .map(p => (
                  <Box key={p.param_name} sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 1 }}>
                    <TextField
                      label={p.param_name}
                      size="small"
                      fullWidth
                      value={addParamValues[p.param_name] || ''}
                      onChange={(e) => setAddParamValues(prev => ({ ...prev, [p.param_name]: e.target.value }))}
                    />
                    <FormControlLabel
                      control={
                        <Checkbox
                          size="small"
                          checked={!!addParamRequired[p.param_name]}
                          onChange={(e) => setAddParamRequired(prev => ({ ...prev, [p.param_name]: e.target.checked }))}
                        />
                      }
                      label="Required"
                      sx={{ whiteSpace: 'nowrap' }}
                    />
                  </Box>
                ))
            ) : (
              <Typography variant="body2" color="text.secondary">
                No writable parameters for this object.
              </Typography>
            )}
          </DialogContent>
          <DialogActions>
            <Button onClick={closeAddDialog}>Cancel</Button>
            <Button variant="contained" onClick={submitAddInstance} disabled={addDialog.params === null}>Add</Button>
          </DialogActions>
        </Dialog>
      )}

      {addResult && (
        <Dialog
          open
          onClose={() => setAddResult(null)}
          slotProps={{ backdrop: { sx: { bgcolor: (theme) => theme.palette.mode === 'dark' ? 'rgba(0,0,0,0.5)' : 'rgba(255,255,255,0.5)' } } }}
          fullWidth
          maxWidth="md"
          scroll="paper"
        >
          <DialogTitle sx={{ color: addResult.status === 'success' ? 'success.main' : 'error.main' }}>
            {addResult.status === 'success' ? 'Success' : addResult.status === 'failure' ? 'Failure' : 'Error'}
          </DialogTitle>
          <DialogContent dividers>
            <Typography variant="body1" sx={{ mb: 1.5, fontWeight: 500 }}>
              {addResult.message}
            </Typography>
            <pre style={{
              margin: 0,
              padding: '12px',
              backgroundColor: theme.palette.action.hover,
              borderRadius: '4px',
              overflow: 'auto',
              fontSize: '13px',
              lineHeight: 1.5,
            }}>
              {(() => {
                const json = JSON.stringify(addResult.result, null, 2);
                if (addResult.failedParams.size === 0) return json;
                return json.split('\n').map((line, i) => {
                  const isFailedParam = [...addResult.failedParams].some(p => line.includes(`"${p}"`));
                  return (
                    <span key={i} style={isFailedParam ? { color: '#d32f2f', fontWeight: 600 } : undefined}>
                      {line}{'\n'}
                    </span>
                  );
                });
              })()}
            </pre>
          </DialogContent>
          <DialogActions>
            <Button onClick={() => setAddResult(null)}>OK</Button>
          </DialogActions>
        </Dialog>
      )}

      <Dialog
        open={errorModal}
        onClose={() => { setErrorModalText(""); setErrorModal(false); setErrorModalTitle("Response"); }}
        slotProps={{ backdrop: { sx: { bgcolor: (theme) => theme.palette.mode === 'dark' ? 'rgba(0,0,0,0.5)' : 'rgba(255,255,255,0.5)' } } }}
        fullWidth
        maxWidth="md"
        scroll="paper"
      >
        <DialogTitle>
          <Box display="flex" alignItems="center">
            <Box flexGrow={1}>{errorModalTitle}</Box>
            <Box>
              <IconButton onClick={() => { setErrorModalText(""); setErrorModal(false); setErrorModalTitle("Response"); }}>
                <SvgIcon><XMarkIcon /></SvgIcon>
              </IconButton>
            </Box>
          </Box>
        </DialogTitle>
        <DialogContent dividers>
          <Box
            component="pre"
            tabIndex={-1}
            sx={{
              m: 0,
              color: 'text.secondary',
              fontFamily: 'inherit',
              fontSize: '0.875rem',
              whiteSpace: 'pre-wrap',
              wordBreak: 'break-word',
            }}
          >
            {errorModalText}
          </Box>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => { setErrorModalText(""); setErrorModal(false); setErrorModalTitle("Response"); }}>OK</Button>
        </DialogActions>
      </Dialog>

      {deviceCommandToExecute && (
        <Dialog
          open={openCommandDialog}
          onClose={() => setOpenCommandDialog(false)}
          slotProps={{ backdrop: { sx: { bgcolor: (theme) => theme.palette.mode === 'dark' ? 'rgba(0,0,0,0.5)' : 'rgba(255,255,255,0.5)' } } }}
          fullWidth
          maxWidth="md"
          scroll="paper"
        >
          <DialogTitle>{Object.keys(deviceCommandToExecute)[0]}</DialogTitle>
          <DialogContent dividers>
            {(() => {
              const cmdKey = Object.keys(deviceCommandToExecute)[0];
              const args = deviceCommandToExecute[cmdKey].input_arg_names;
              if (!Array.isArray(args) || args.length === 0) return null;
              return (
                <>
                  <DialogContentText tabIndex={-1} sx={{ mb: 1.5 }}>Input Arguments:</DialogContentText>
                  {args.map(arg => (
                    <TextField
                      key={arg}
                      autoFocus
                      margin="dense"
                      id={arg}
                      label={arg}
                      type="text"
                      fullWidth
                      sx={{ mb: 1.5 }}
                      onChange={(e) => setInputArgsValue(prev => ({ ...prev, [arg]: e.target.value }))}
                      value={inputArgsValue[arg] || ''}
                    />
                  ))}
                </>
              );
            })()}
          </DialogContent>
          <DialogActions>
            <Button onClick={() => {
              setInputArgsValue({});
              setDeviceCommandToExecute(null);
              setOpenCommandDialog(false);
            }}>Cancel</Button>
            <Button onClick={applyCommand}>Apply</Button>
          </DialogActions>
        </Dialog>
      )}

      <Backdrop
        sx={{ color: '#fff', zIndex: (theme) => theme.zIndex.drawer + 1, overflow: 'hidden' }}
        open={showLoading}
      >
        <CircularProgress />
      </Backdrop>
    </Card>
  );
};