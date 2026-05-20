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
  Tooltip,
} from '@mui/material';
import ArrowRightIcon from '@heroicons/react/24/solid/ArrowRightIcon';
import CircularProgress from '@mui/material/CircularProgress';
import ArrowPathIcon from '@heroicons/react/24/outline/ArrowPathIcon';
import ArrowUturnLeftIcon from '@heroicons/react/24/outline/ArrowUturnLeftIcon';
import Pencil from '@heroicons/react/24/outline/PencilIcon';
import { useRouter } from 'next/router';
import { useBackendContext } from 'src/contexts/backend-context';

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

export const DevicesDiscovery = ({ onStatusRefresh }) => {
  const router = useRouter();
  const { httpRequest, apiPrefix } = useBackendContext();

  const deviceID = router.query.id?.[0];
  const pathSegments = router.query.id?.slice(2) || [];
  const pathKey = pathSegments.join('/');
  const currentPath = useMemo(() => segmentsToPath(pathSegments), [pathKey]);

  const [rootPath, setRootPath] = useState(null);
  const [dataModel, setDataModel] = useState('');
  const [children, setChildren] = useState([]);
  const [parameters, setParameters] = useState([]);
  const [showLoading, setShowLoading] = useState(false);
  const [errorText, setErrorText] = useState('');
  const [editOpen, setEditOpen] = useState(false);
  const [editName, setEditName] = useState('');
  const [editValue, setEditValue] = useState('');

  const navigateTo = useCallback((path) => {
    const urlPath = pathToUrl(path);
    router.push(
      `/devices/cwmp/${deviceID}/discovery/${urlPath}`,
      undefined,
      { shallow: true }
    );
  }, [deviceID, router]);

  // Walk one level up. We intentionally pop a single segment even when the
  // current parent is a numeric instance (e.g. WiFi.Radio.1.Stats. -> Radio.1.):
  // an extra pop would skip past the instance row in one click, which is not
  // what the user expects from a single "back" press.
  const navigateBack = useCallback(() => {
    const parts = currentPath.replace(/\.$/, '').split('.');
    if (parts.length <= 1) return;
    parts.pop();
    navigateTo(parts.join('.') + '.');
  }, [currentPath, navigateTo]);

  const loadPath = useCallback(async (path) => {
    if (!deviceID || !path) return;
    onStatusRefresh?.();
    setShowLoading(true);
    setErrorText('');
    try {
      const body = JSON.stringify({ path, fetch_values: true });
      const { status, result } = await httpRequest(
        `${apiPrefix}/device/cwmp/${deviceID}/parameters`,
        'PUT',
        body
      );
      if (status === 200 && result) {
        setChildren(result.children || []);
        setParameters(result.parameters || []);
        if (result.data_model) setDataModel(result.data_model);
        return;
      }
      setErrorText('Failed to load parameters from device');
    } catch {
      setErrorText('Failed to load parameters from device');
    } finally {
      setShowLoading(false);
    }
  }, [apiPrefix, deviceID, httpRequest, onStatusRefresh]);

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
  }, [deviceID]);

  useEffect(() => {
    if (!deviceID) return;
    const path = currentPath || rootPath;
    if (path) loadPath(path);
  }, [currentPath, deviceID, rootPath, loadPath]);

  const applyEdit = async () => {
    setEditOpen(false);
    setShowLoading(true);
    try {
      const body = JSON.stringify({ values: { [editName]: editValue } });
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

  const browsePath = currentPath || rootPath || '';
  const isRoot = browsePath && rootPath && browsePath === rootPath;

  return (
    <Card>
      <CardContent>
        {dataModel && (
          <Alert severity="info" sx={{ mb: 2 }}>
            Data model: {dataModel}
          </Alert>
        )}
        {errorText && (
          <Alert severity="error" sx={{ mb: 2 }} onClose={() => setErrorText('')}>
            {errorText}
          </Alert>
        )}

        <Box sx={{ display: 'flex', alignItems: 'center', mb: 2, gap: 1 }}>
          {!isRoot && (
            <IconButton onClick={navigateBack} size="small">
              <SvgIcon><ArrowUturnLeftIcon /></SvgIcon>
            </IconButton>
          )}
          <Typography variant="subtitle1" sx={{ fontFamily: 'monospace', flexGrow: 1 }}>
            {browsePath || '...'}
          </Typography>
          <Tooltip title="Refresh">
            <IconButton onClick={() => loadPath(browsePath)} disabled={!browsePath}>
              <SvgIcon><ArrowPathIcon /></SvgIcon>
            </IconButton>
          </Tooltip>
        </Box>

        <List dense>
          {children.map((child) => (
            <ListItem
              key={child.name}
              divider
              sx={{ cursor: 'pointer', '&:hover': { bgcolor: 'action.hover' } }}
              onClick={() => navigateTo(child.name)}
              secondaryAction={
                <IconButton onClick={(e) => { e.stopPropagation(); navigateTo(child.name); }}>
                  <SvgIcon><ArrowRightIcon /></SvgIcon>
                </IconButton>
              }
            >
              <ListItemText
                primary={<b>{getDisplayName(child.name, browsePath) + '.'}</b>}
                secondary={child.writable ? 'writable object' : 'object'}
              />
            </ListItem>
          ))}

          {parameters.map((param) => (
            <ListItem
              key={param.name}
              divider
              sx={param.writable ? { cursor: 'pointer', '&:hover': { bgcolor: 'action.hover' } } : {}}
              onClick={param.writable ? () => {
                setEditName(param.name);
                setEditValue(param.value ?? '');
                setEditOpen(true);
              } : undefined}
              secondaryAction={
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, maxWidth: '50%' }}>
                  <Typography variant="body2" color="text.secondary" noWrap title={param.value}>
                    {param.value ?? '-'}
                  </Typography>
                  {param.writable && (
                    <IconButton
                      size="small"
                      onClick={(e) => {
                        e.stopPropagation();
                        setEditName(param.name);
                        setEditValue(param.value ?? '');
                        setEditOpen(true);
                      }}
                    >
                      <SvgIcon fontSize="small"><Pencil /></SvgIcon>
                    </IconButton>
                  )}
                </Box>
              }
            >
              <ListItemText primary={getDisplayName(param.name, browsePath)} />
            </ListItem>
          ))}

          {!showLoading && children.length === 0 && parameters.length === 0 && browsePath && (
            <ListItem>
              <ListItemText primary="No child parameters at this level" />
            </ListItem>
          )}
        </List>

        <Backdrop open={showLoading} sx={{ zIndex: (theme) => theme.zIndex.drawer + 1 }}>
          <CircularProgress color="inherit" />
        </Backdrop>

        <Dialog open={editOpen} onClose={() => setEditOpen(false)} maxWidth="sm" fullWidth>
          <DialogTitle>Set parameter</DialogTitle>
          <DialogContent>
            <DialogContentText sx={{ mb: 2, fontFamily: 'monospace', wordBreak: 'break-all' }}>
              {editName}
            </DialogContentText>
            <TextField
              fullWidth
              label="Value"
              value={editValue}
              onChange={(e) => setEditValue(e.target.value)}
            />
          </DialogContent>
          <DialogActions>
            <Button onClick={() => setEditOpen(false)}>Cancel</Button>
            <Button variant="contained" onClick={applyEdit}>Apply</Button>
          </DialogActions>
        </Dialog>
      </CardContent>
    </Card>
  );
};
