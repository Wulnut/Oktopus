import { useEffect, useState, useCallback, useMemo } from 'react';
import {
  Card,
  CardContent,
  SvgIcon,
  IconButton,
  List,
  ListItem,
  ListItemText,
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
  Fab,
  Tooltip,
} from '@mui/material';
import ArrowRightIcon from '@heroicons/react/24/solid/ArrowRightIcon';
import CircularProgress from '@mui/material/CircularProgress';
import ArrowPathIcon from '@heroicons/react/24/outline/ArrowPathIcon';
import PlusCircleIcon from '@heroicons/react/24/outline/PlusCircleIcon';
import Pencil from "@heroicons/react/24/outline/PencilIcon";
import ArrowUturnLeftIcon from '@heroicons/react/24/outline/ArrowUturnLeftIcon';
import XMarkIcon from '@heroicons/react/24/outline/XMarkIcon';
import TrashIcon from '@heroicons/react/24/outline/TrashIcon';
import PlayCircleIcon from '@heroicons/react/24/outline/PlayCircleIcon';
import { useRouter } from 'next/router';

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

// Convert TR-181 dot path to URL segments: "Device.WiFi.Radio.1." -> "Device/WiFi/Radio/1"
const pathToUrl = (tr181Path) => tr181Path.replace(/\.$/, '').replaceAll('.', '/');

// Convert URL segments back to TR-181 dot path: ["Device","WiFi","Radio","1"] -> "Device.WiFi.Radio.1."
const segmentsToPath = (segments) => segments.length > 0 ? segments.join('.') + '.' : 'Device.';

// Sort instance keys by their numeric segments (1,2,10 not 1,10,2)
const sortInstanceKeys = (a, b) => {
  const numsA = a.match(/\d+/g)?.map(Number) || [];
  const numsB = b.match(/\d+/g)?.map(Number) || [];
  for (let i = 0; i < Math.min(numsA.length, numsB.length); i++) {
    if (numsA[i] !== numsB[i]) return numsA[i] - numsB[i];
  }
  return numsA.length - numsB.length;
};

// Extract the last meaningful name from a supported_obj_path
// "Device.Bridging.Bridge.{i}." -> "Bridge"
const getObjName = (path) => {
  const parts = path.replace(/\.$/, '').split('.');
  // Walk backward to find a non-{i} segment
  for (let i = parts.length - 1; i >= 0; i--) {
    if (parts[i] !== '{i}') return parts[i];
  }
  return parts[parts.length - 1];
};

// Get relative child path from parent template
// parent: "Device.Bridging.Bridge.{i}."  child: "Device.Bridging.Bridge.{i}.Port.{i}." -> "Port.{i}."
const getRelativeChildPath = (childPath, parentPath) => {
  if (childPath.startsWith(parentPath)) {
    return childPath.substring(parentPath.length);
  }
  return childPath;
};

// Check if child is a direct child (one object level deeper) of parent
const isDirectChild = (childPath, parentPath) => {
  const rel = getRelativeChildPath(childPath, parentPath);
  if (!rel) return false;
  // Direct child: "Port.{i}." or "Stats." — at most one non-{i} name segment
  const parts = rel.replace(/\.$/, '').split('.');
  const nameSegments = parts.filter(p => p !== '{i}');
  return nameSegments.length === 1;
};

// Get display name for a child object relative to parent
// "Port.{i}." -> "Port"
const getChildDisplayName = (relativePath) => {
  const parts = relativePath.replace(/\.$/, '').split('.');
  return parts.find(p => p !== '{i}') || parts[0];
};

// Check if a relative child path contains {i} (multi-instance)
const isMultiInstance = (relativePath) => relativePath.includes('{i}');

// Extract command name from path: "Device.X.Reset()" -> "Reset"
const extractCommandName = (commandPath) => {
  if (!commandPath) return '';
  const parts = commandPath.split('.');
  return parts[parts.length - 1].replace(/\(\)$/, '');
};

// Generate unique command_key: "Reset_20250105_143022_a3f2"
const generateUniqueCommandKey = (commandPath) => {
  const commandName = extractCommandName(commandPath);
  if (!commandName) return '';
  const now = new Date();
  const pad = (n) => String(n).padStart(2, '0');
  const randomHex = Math.floor(Math.random() * 0x10000).toString(16).padStart(4, '0');
  return `${commandName}_${now.getFullYear()}${pad(now.getMonth()+1)}${pad(now.getDate())}_${pad(now.getHours())}${pad(now.getMinutes())}${pad(now.getSeconds())}_${randomHex}`;
};

// Build auth headers
const getAuthHeaders = () => {
  const headers = new Headers();
  headers.append("Content-Type", "application/json");
  headers.append("Authorization", localStorage.getItem("token"));
  return headers;
};

export const DevicesDiscovery = () => {
  const router = useRouter();

  // Derive device ID and current TR-181 path from URL
  const deviceID = router.query.id?.[0];
  const pathSegments = router.query.id?.slice(2) || [];
  const pathKey = pathSegments.join('/');
  // eslint-disable-next-line react-hooks/exhaustive-deps
  const currentPath = useMemo(() => segmentsToPath(pathSegments), [pathKey]);

  // State
  const [deviceParameters, setDeviceParameters] = useState(null);
  const [deviceParametersValue, setDeviceParametersValue] = useState({});
  const [childObjects, setChildObjects] = useState([]);
  const [showLoading, setShowLoading] = useState(false);
  const [open, setOpen] = useState(false);
  const [parameter, setParameter] = useState(null);
  const [parameterValue, setParameterValue] = useState(null);
  const [parameterValueChange, setParameterValueChange] = useState(null);
  const [errorModal, setErrorModal] = useState(false);
  const [errorModalText, setErrorModalText] = useState("");
  const [deviceOfflineError, setDeviceOfflineError] = useState(false);
  const [deviceOfflineErrorText, setDeviceOfflineErrorText] = useState("");
  const [openCommandDialog, setOpenCommandDialog] = useState(false);
  const [deviceCommandToExecute, setDeviceCommandToExecute] = useState(null);
  const [inputArgsValue, setInputArgsValue] = useState({});

  // Navigate to a TR-181 path by updating the URL
  const navigateTo = useCallback((tr181Path) => {
    const urlPath = pathToUrl(tr181Path);
    router.push(
      `/devices/usp/${deviceID}/discovery/${urlPath}`,
      undefined,
      { shallow: true }
    );
  }, [deviceID, router]);

  // Navigate up one level
  const navigateBack = useCallback(() => {
    const parts = currentPath.replace(/\.$/, '').split('.');
    if (parts.length <= 1) return; // Already at Device.

    // Remove last segment(s): skip trailing numbers/wildcards to go up to parent object level
    let pathsToRemove = 1;
    if (/^\d+$/.test(parts[parts.length - 1]) || parts[parts.length - 1] === '*') {
      pathsToRemove = 2;
    }
    parts.splice(parts.length - pathsToRemove, pathsToRemove);

    // If the resulting path ends with a concrete instance number, replace it with *
    // so we go back to the multi-instance view (e.g., Bridge.1. -> Bridge.*.)
    if (parts.length > 1 && /^\d+$/.test(parts[parts.length - 1])) {
      parts[parts.length - 1] = '*';
    }

    const parentPath = parts.join('.') + '.';
    navigateTo(parentPath);
  }, [currentPath, navigateTo]);

  // API helpers
  const fetchWithAuth = useCallback(async (endpoint, body) => {
    const result = await fetch(
      `${process.env.NEXT_PUBLIC_REST_ENDPOINT || ""}/api/device/${deviceID}/any/${endpoint}`,
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
      } catch (e) { /* use raw text */ }
      errorText = errorText.trim().replace(/^["']|["']$/g, '');
      throw new Error(errorText || `Request failed with status ${result.status}`);
    }
    return result.json();
  }, [deviceID, router]);

  // Convert all instance numbers AND wildcards to {i} for template matching
  const toTemplatePath = (path) => {
    return path.split('.').map(seg => /^\d+$/.test(seg) || seg === '*' ? '{i}' : seg).join('.');
  };

  // Main data fetching function
  const updateDeviceParameters = useCallback(async (path, { preserveState = false } = {}) => {
    setShowLoading(true);
    if (!preserveState) {
      setDeviceParameters(null);
      setDeviceParametersValue({});
      setChildObjects([]);
    }

    try {
      // Use path directly for USP queries — it already has * where needed
      // GetSupportedDM accepts both concrete (Bridge.2.) and wildcard (Bridge.*.) paths
      const content = await fetchWithAuth('parameters', {
        obj_paths: [path],
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

      // The first supported_obj is the queried object itself (has params/commands)
      // Remaining are child objects (sub-objects to drill into)
      const mainObj = supportedObjs[0];
      const children = supportedObjs.slice(1).filter(
        child => isDirectChild(child.supported_obj_path, mainObj.supported_obj_path)
      );

      // Sort children alphabetically by name
      children.sort((a, b) => getObjName(a.supported_obj_path).localeCompare(getObjName(b.supported_obj_path)));
      setChildObjects(children);

      // 2. Fetch parameter values if the main object has params
      const supportedParams = mainObj.supported_params;
      if (supportedParams?.length) {
        // Reconstruct the concrete query path: map {i} in template back to
        // the actual numbers/* from the input path
        const templateParts = mainObj.supported_obj_path.split('.');
        const inputParts = path.split('.');
        const concreteObjPath = templateParts.map((seg, idx) => {
          if (seg === '{i}' && idx < inputParts.length) return inputParts[idx];
          return seg;
        }).join('.');
        const paramsToFetch = supportedParams.map(p => concreteObjPath + p.param_name);

        const paramsInfo = {};
        supportedParams.forEach(p => {
          paramsInfo[p.param_name] = {
            value_change: p.value_change,
            value_type: p.value_type,
            access: p.access,
            value: "-",
          };
        });

        const result = await fetchWithAuth('get', {
          param_paths: paramsToFetch,
          max_depth: 1,
        });

        if (result?.req_path_results) {
          const values = {};
          result.req_path_results.forEach(x => {
            if (!x.resolved_path_results) {
              values[x.requested_path] = {};
              return;
            }

            const parts = x.requested_path.split('.');
            if (parts[parts.length - 2] === '*') {
              // Multi-instance: group by resolved path (instance)
              x.resolved_path_results.forEach(y => {
                const key = Object.keys(y.result_params)[0];
                if (!values[y.resolved_path]) values[y.resolved_path] = [];
                const val = y.result_params[key] === "" ? '""' : y.result_params[key];
                values[y.resolved_path].push({
                  [key]: { ...paramsInfo[key], value: val }
                });
              });
            } else {
              // Single instance: flat key-value
              Object.keys(x.resolved_path_results[0].result_params).forEach(key => {
                const val = x.resolved_path_results[0].result_params[key];
                values[key] = {
                  ...paramsInfo[key],
                  value: val === "" ? '""' : val,
                };
              });
            }
          });

          setDeviceParametersValue(values);
        }
      }

      setDeviceParameters(content);
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
  }, [fetchWithAuth]);

  // Fetch data when path changes
  useEffect(() => {
    if (deviceID && currentPath) {
      updateDeviceParameters(currentPath);
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [currentPath, deviceID]);

  // Add device object instance
  const addDeviceObj = async (objPath) => {
    setShowLoading(true);
    try {
      const result = await fetchWithAuth('add', {
        allow_partial: true,
        create_objs: [{ obj_path: objPath }],
      });
      if (result) {
        // Re-fetch current view
        updateDeviceParameters(currentPath);
      }
    } catch (error) {
      setErrorModalText(error.message);
      setErrorModal(true);
    } finally {
      setShowLoading(false);
    }
  };

  // Delete device object instance
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

  // Set parameter value
  const applyParameterChange = async () => {
    const params = parameter.split('.');
    const parameterToChange = params.pop();
    const objToChange = params.join('.');

    setOpen(false);
    setShowLoading(true);

    try {
      const result = await fetchWithAuth('set', {
        allow_partial: true,
        update_objs: [{
          obj_path: objToChange,
          param_settings: [{
            param: parameterToChange,
            value: parameterValueChange,
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

      // Update value in state
      if (/^\d+$/.test(params[params.length - 1])) {
        // Multi-instance param
        setDeviceParametersValue(prev => ({
          ...prev,
          [objToChange + "."]: prev[objToChange + "."]?.map(el => {
            if (el[parameterToChange] !== undefined) {
              return { ...el, [parameterToChange]: { ...el[parameterToChange], value: parameterValueChange } };
            }
            return el;
          }),
        }));
      } else {
        setDeviceParametersValue(prev => ({
          ...prev,
          [parameterToChange]: { ...prev[parameterToChange], value: parameterValueChange },
        }));
      }
    } catch (error) {
      setErrorModalText(error.message);
      setErrorModal(true);
    } finally {
      setShowLoading(false);
    }
  };

  // Execute command
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
        const operationResp = result.operation_results?.[0]?.OperationResp;
        if (operationResp?.CmdFailure) {
          setErrorModalText(JSON.stringify(result, null, 2));
          setErrorModal(true);
        } else {
          setErrorModalText(JSON.stringify(result, null, 2));
          setErrorModal(true);
        }
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

  // Show parameter edit dialog
  const showEditDialog = (param, paramValue) => {
    setParameter(param);
    setParameterValue(paramValue === '""' ? "" : paramValue);
    setOpen(true);
  };

  // Render the main object header with back button
  const renderObjectHeader = () => {
    if (!deviceParameters?.req_obj_results?.[0]) return null;
    const mainObj = deviceParameters.req_obj_results[0].supported_objs[0];
    const isRoot = currentPath === 'Device.';

    return (
      <List dense>
        <ListItem
          divider
          secondaryAction={
            !isRoot && (
              <IconButton onClick={navigateBack}>
                <SvgIcon><ArrowUturnLeftIcon /></SvgIcon>
              </IconButton>
            )
          }
          sx={{ boxShadow: 'rgba(149, 157, 165, 0.2) 0px 0px 5px;' }}
        >
          <ListItemText
            primary={<b>{currentPath}</b>}
            sx={{ fontWeight: 'bold' }}
          />
        </ListItem>
      </List>
    );
  };

  // Render child object buttons for a given instance path
  const renderChildObjects = (instancePath, mainObj) => {
    if (childObjects.length === 0) return null;

    const sortedChildren = [...childObjects].sort((a, b) =>
      getObjName(a.supported_obj_path).localeCompare(getObjName(b.supported_obj_path))
    );

    return sortedChildren.map(child => {
      const relPath = getRelativeChildPath(child.supported_obj_path, mainObj.supported_obj_path);
      const displayName = getChildDisplayName(relPath);
      const isMulti = isMultiInstance(relPath);

      // Build the concrete navigation path
      // instancePath = "Device.Bridging.Bridge.1."
      // relPath = "Port.{i}." -> navigate to "Device.Bridging.Bridge.1.Port.*."
      // relPath = "Stats." -> navigate to "Device.Bridging.Bridge.1.Stats."
      let navPath;
      if (isMulti) {
        navPath = instancePath + relPath.replace('{i}', '*');
      } else {
        navPath = instancePath + relPath;
      }

      const canAdd = child.access === ObjAccessType.AddDelete || child.access === ObjAccessType.AddOnly;
      const addPath = instancePath + relPath.replace('{i}.', '');

      return (
        <List component="div" disablePadding dense key={displayName}>
          <ListItem
            divider
            sx={{ boxShadow: 'rgba(149, 157, 165, 0.2) 0px 0px 5px;', pl: 4 }}
            secondaryAction={
              <Box sx={{ display: 'flex', alignItems: 'center' }}>
                {canAdd && (
                  <IconButton onClick={() => addDeviceObj(addPath)}>
                    <SvgIcon><PlusCircleIcon /></SvgIcon>
                  </IconButton>
                )}
                <IconButton onClick={() => navigateTo(navPath)}>
                  <SvgIcon><ArrowRightIcon /></SvgIcon>
                </IconButton>
              </Box>
            }
          >
            <ListItemText primary={<b>{displayName + (isMulti ? '.{i}.' : '.')}</b>} />
          </ListItem>
        </List>
      );
    });
  };

  // Sort params: alphabetically, but NumberOfEntries params go last
  const sortParams = (params, getName = (p) => p) => {
    return [...params].sort((a, b) => {
      const nameA = getName(a);
      const nameB = getName(b);
      const aIsCount = nameA.endsWith('NumberOfEntries');
      const bIsCount = nameB.endsWith('NumberOfEntries');
      if (aIsCount !== bIsCount) return aIsCount ? 1 : -1;
      return nameA.localeCompare(nameB);
    });
  };

  // Render parameters for a single (non-instance) object
  const renderFlatParams = (mainObj) => {
    if (!mainObj.supported_params?.length) return null;

    const sortedParams = sortParams(mainObj.supported_params, p => p.param_name);

    return sortedParams.map(p => (
      <List component="div" disablePadding dense key={p.param_name}>
        <ListItem
          divider
          sx={{ boxShadow: 'rgba(149, 157, 165, 0.2) 0px 0px 5px;', pl: 4 }}
          secondaryAction={
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
              <Typography variant="body2" color="text.secondary" sx={{ maxWidth: 400, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                {deviceParametersValue[p.param_name]?.value}
              </Typography>
              {deviceParametersValue[p.param_name]?.access > ParamAccessType.ReadOnly && (
                <IconButton onClick={() => showEditDialog(
                  mainObj.supported_obj_path + p.param_name,
                  deviceParametersValue[p.param_name]?.value
                )}>
                  <SvgIcon sx={{ width: '20px' }}><Pencil /></SvgIcon>
                </IconButton>
              )}
            </Box>
          }
        >
          <ListItemText primary={p.param_name} />
        </ListItem>
      </List>
    ));
  };

  // Render commands for a single object (or per-instance)
  const renderCommands = (commands, pathPrefix) => {
    if (!commands?.length) return null;

    const sortedCommands = [...commands].sort((a, b) =>
      a.command_name.localeCompare(b.command_name)
    );

    return sortedCommands.map(cmd => (
      <List component="div" disablePadding dense key={cmd.command_name + '__' + pathPrefix}>
        <ListItem
          divider
          sx={{ boxShadow: 'rgba(149, 157, 165, 0.2) 0px 0px 5px;', pl: 4 }}
          secondaryAction={
            <IconButton onClick={() => {
              setDeviceCommandToExecute({
                [pathPrefix + cmd.command_name]: { input_arg_names: cmd.input_arg_names }
              });
              setOpenCommandDialog(true);
            }}>
              <SvgIcon><PlayCircleIcon /></SvgIcon>
            </IconButton>
          }
        >
          <ListItemText primary={cmd.command_name} />
        </ListItem>
      </List>
    ));
  };

  // Render instance-based parameters (multi-instance objects)
  const renderInstanceParams = (mainObj) => {
    const templatePath = mainObj.supported_obj_path;
    const instancePattern = new RegExp(
      '^' + templatePath.replace(/\{i\}/g, '\\d+').replace(/\./g, '\\.') + '$'
    );

    const instanceKeys = Object.keys(deviceParametersValue)
      .filter(key => instancePattern.test(key))
      .sort(sortInstanceKeys);

    if (instanceKeys.length === 0) return null;

    const access = mainObj.access;
    const canDelete = access === ObjAccessType.AddDelete || access === ObjAccessType.DeleteOnly;

    return instanceKeys.map(instanceKey => {
      const params = deviceParametersValue[instanceKey] || [];

      // Sort params: alphabetically, NumberOfEntries last
      const sortedParams = sortParams(params, p => Object.keys(p)[0]);

      return (
        <List dense key={instanceKey}>
          <ListItem
            divider
            sx={{ boxShadow: 'rgba(149, 157, 165, 0.2) 0px 0px 5px;', pl: 2, backgroundColor: 'rgba(0,0,0,0.02)' }}
            secondaryAction={
              canDelete && (
                <IconButton onClick={() => deleteDeviceObj(instanceKey)}>
                  <SvgIcon><TrashIcon /></SvgIcon>
                </IconButton>
              )
            }
          >
            <ListItemText primary={<b>{instanceKey}</b>} />
          </ListItem>

          {/* Parameters for this instance */}
          {sortedParams.map(param => {
            const paramName = Object.keys(param)[0];
            const paramData = param[paramName];
            return (
              <List component="div" disablePadding dense key={paramName}>
                <ListItem
                  divider
                  sx={{ boxShadow: 'rgba(149, 157, 165, 0.2) 0px 0px 5px;', pl: 4 }}
                  secondaryAction={
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
                      <Typography variant="body2" color="text.secondary" sx={{ maxWidth: 400, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                        {paramData.value}
                      </Typography>
                      {paramData.access > ParamAccessType.ReadOnly && (
                        <IconButton onClick={() => showEditDialog(instanceKey + paramName, paramData.value)}>
                          <SvgIcon sx={{ width: '20px' }}><Pencil /></SvgIcon>
                        </IconButton>
                      )}
                    </Box>
                  }
                >
                  <ListItemText primary={paramName} />
                </ListItem>
              </List>
            );
          })}

          {/* Commands for this instance */}
          {renderCommands(mainObj.supported_commands, instanceKey)}

          {/* Child objects for this instance */}
          {renderChildObjects(instanceKey, mainObj)}
        </List>
      );
    });
  };

  // Render non-instance child objects at the top level (e.g., Device. showing WiFi., DeviceInfo., etc.)
  const renderTopLevelChildObjects = () => {
    if (!deviceParameters?.req_obj_results?.[0]) return null;
    const allObjs = deviceParameters.req_obj_results[0].supported_objs;
    const mainObj = allObjs[0];

    // Check if the object itself ends with {i} (is multi-instance)
    const pathParts = mainObj.supported_obj_path.replace(/\.$/, '').split('.');
    const isInstanceObj = pathParts[pathParts.length - 1] === '{i}';
    const hasInstanceValues = Object.keys(deviceParametersValue).some(k => k.includes('.'));

    if (isInstanceObj && hasInstanceValues) {
      // Instance objects: children are rendered per-instance in renderInstanceParams
      return null;
    }

    // Non-instance (or concrete single instance): render children as top-level drill-down buttons
    return renderChildObjects(currentPath, mainObj);
  };

  // Main render logic
  const showParameters = () => {
    if (!deviceParameters?.req_obj_results?.length) return null;

    const mainObj = deviceParameters.req_obj_results[0].supported_objs[0];
    // Check if the object itself is multi-instance (ends with {i}.)
    // e.g., "Device.Bridge.{i}." is multi-instance, but "Device.Bridge.{i}.Stats." is not
    const pathParts = mainObj.supported_obj_path.replace(/\.$/, '').split('.');
    const isInstanceObj = pathParts[pathParts.length - 1] === '{i}';

    // Determine if values were stored flat (by param name) or by instance key
    // If the concrete query path had no wildcard, values are flat even for objects
    // whose template contains {i} in ancestor segments
    const hasInstanceValues = Object.keys(deviceParametersValue).some(k => k.includes('.'));
    const showAsInstance = isInstanceObj && hasInstanceValues;

    return (
      <>
        {renderObjectHeader()}

        {/* For non-instance objects: show flat params, commands, then child objects */}
        {!showAsInstance && renderFlatParams(mainObj)}
        {!showAsInstance && renderCommands(mainObj.supported_commands,
          currentPath.endsWith('.') ? currentPath : mainObj.supported_obj_path)}
        {renderTopLevelChildObjects()}

        {/* For instance objects: show per-instance params, commands, and child objects */}
        {showAsInstance && renderInstanceParams(mainObj)}
      </>
    );
  };

  // Loading state
  if (!deviceParameters && !errorModal && !deviceOfflineError) {
    return (
      <Box sx={{ display: 'flex', justifyContent: 'center' }}>
        <CircularProgress />
      </Box>
    );
  }

  return (
    <Card>
      <CardContent>
        {deviceOfflineError && deviceOfflineErrorText && (
          <Alert
            severity="error"
            onClose={() => { setDeviceOfflineError(false); setDeviceOfflineErrorText(""); }}
            sx={{ mb: 2 }}
          >
            {deviceOfflineErrorText}
          </Alert>
        )}
        {showParameters()}
      </CardContent>

      {/* Parameter Edit Dialog */}
      <Dialog open={open} slotProps={{ backdrop: { style: { backgroundColor: 'rgba(255,255,255,0.5)' } } }}>
        <DialogContent>
          <DialogContentText>{parameter}</DialogContentText>
          <TextField
            autoFocus
            margin="dense"
            id="parameterValue"
            fullWidth
            variant="standard"
            defaultValue={parameterValue}
            autoComplete="off"
            onChange={(e) => setParameterValueChange(e.target.value)}
          />
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setOpen(false)}>Cancel</Button>
          <Button onClick={applyParameterChange}>Apply</Button>
        </DialogActions>
      </Dialog>

      {/* Error/Response Modal */}
      <Dialog
        open={errorModal}
        slotProps={{ backdrop: { style: { backgroundColor: 'rgba(255,255,255,0.5)' } } }}
        fullWidth
        maxWidth="md"
        scroll="paper"
      >
        <DialogTitle>
          <Box display="flex" alignItems="center">
            <Box flexGrow={1}>Response</Box>
            <Box>
              <IconButton onClick={() => { setErrorModalText(""); setErrorModal(false); }}>
                <SvgIcon><XMarkIcon /></SvgIcon>
              </IconButton>
            </Box>
          </Box>
        </DialogTitle>
        <DialogContent dividers>
          <DialogContentText tabIndex={-1}>
            <pre style={{ color: 'black' }}>{errorModalText}</pre>
          </DialogContentText>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => { setErrorModalText(""); setErrorModal(false); }}>OK</Button>
        </DialogActions>
      </Dialog>

      {/* Command Execution Dialog */}
      {deviceCommandToExecute && (
        <Dialog
          open={openCommandDialog}
          slotProps={{ backdrop: { style: { backgroundColor: 'rgba(255,255,255,0.5)' } } }}
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
                  <DialogContentText tabIndex={-1}>Input Arguments:</DialogContentText>
                  {args.map(arg => (
                    <TextField
                      key={arg}
                      autoFocus
                      margin="dense"
                      id={arg}
                      label={arg}
                      type="text"
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

      <Tooltip title="Refresh">
        <Fab
          color="primary"
          size="small"
          disabled={showLoading}
          onClick={() => updateDeviceParameters(currentPath, { preserveState: true })}
          sx={{ position: 'fixed', bottom: 24, right: 24 }}
        >
          <SvgIcon fontSize="small"><ArrowPathIcon /></SvgIcon>
        </Fab>
      </Tooltip>
    </Card>
  );
};
