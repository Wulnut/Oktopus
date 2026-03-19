import {
  Box,
  Card,
  Chip,
  IconButton,
  LinearProgress,
  Stack,
  SvgIcon,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Tooltip,
  Typography,
} from '@mui/material';
import PencilIcon from '@heroicons/react/24/solid/PencilIcon';
import PlayIcon from '@heroicons/react/24/solid/PlayIcon';
import TrashIcon from '@heroicons/react/24/solid/TrashIcon';
import ClockIcon from '@heroicons/react/24/solid/ClockIcon';
import DocumentDuplicateIcon from '@heroicons/react/24/solid/DocumentDuplicateIcon';

export const ScriptsTable = ({
  scripts = [],
  loading,
  onEdit,
  onDuplicate,
  onExecute,
  onHistory,
  onDelete,
}) => {
  if (loading) {
    return (
      <Card>
        <LinearProgress />
      </Card>
    );
  }

  if (scripts.length === 0) {
    return (
      <Card sx={{ p: 4, textAlign: 'center' }}>
        <Typography color="text.secondary">
          No scripts yet. Create your first script to automate USP commands.
        </Typography>
      </Card>
    );
  }

  return (
    <Card>
      <TableContainer>
        <Table>
          <TableHead>
            <TableRow>
              <TableCell>Name</TableCell>
              <TableCell>Description</TableCell>
              <TableCell>Steps</TableCell>
              <TableCell>Tags</TableCell>
              <TableCell>Updated</TableCell>
              <TableCell align="right">Actions</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {scripts.map((script) => (
              <TableRow key={script.id} hover>
                <TableCell>
                  <Stack direction="row" alignItems="center" spacing={1}>
                    <Typography variant="subtitle2">{script.name}</Typography>
                    {script.builtin && (
                      <Chip label="Built-in" size="small" color="info" variant="outlined" />
                    )}
                  </Stack>
                </TableCell>
                <TableCell>
                  <Typography variant="body2" color="text.secondary" noWrap sx={{ maxWidth: 300 }}>
                    {script.description || '-'}
                  </Typography>
                </TableCell>
                <TableCell>{script.steps?.length || 0}</TableCell>
                <TableCell>
                  <Stack direction="row" spacing={0.5} flexWrap="wrap" useFlexGap>
                    {(script.tags || []).map((tag) => (
                      <Chip key={tag} label={tag} size="small" variant="outlined" />
                    ))}
                  </Stack>
                </TableCell>
                <TableCell>
                  <Typography variant="body2" color="text.secondary">
                    {script.updated_at
                      ? new Date(script.updated_at).toLocaleDateString()
                      : '-'}
                  </Typography>
                </TableCell>
                <TableCell align="right">
                  <Stack direction="row" spacing={0} justifyContent="flex-end">
                    <Tooltip title="Execute">
                      <IconButton size="small" onClick={() => onExecute(script)} color="success">
                        <SvgIcon fontSize="small"><PlayIcon /></SvgIcon>
                      </IconButton>
                    </Tooltip>
                    <Tooltip title="Edit">
                      <IconButton
                        size="small"
                        onClick={() => onEdit(script)}
                        disabled={script.builtin}
                        color="warning"
                      >
                        <SvgIcon fontSize="small"><PencilIcon /></SvgIcon>
                      </IconButton>
                    </Tooltip>
                    <Tooltip title="Duplicate">
                      <IconButton size="small" onClick={() => onDuplicate(script)} color="warning">
                        <SvgIcon fontSize="small"><DocumentDuplicateIcon /></SvgIcon>
                      </IconButton>
                    </Tooltip>
                    <Tooltip title="History">
                      <IconButton size="small" onClick={() => onHistory(script)} color="warning">
                        <SvgIcon fontSize="small"><ClockIcon /></SvgIcon>
                      </IconButton>
                    </Tooltip>
                    <Tooltip title="Delete">
                      <IconButton
                        size="small"
                        onClick={() => onDelete(script)}
                        disabled={script.builtin}
                      >
                        <SvgIcon fontSize="small" color={script.builtin ? 'disabled' : 'error'}>
                          <TrashIcon />
                        </SvgIcon>
                      </IconButton>
                    </Tooltip>
                  </Stack>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </TableContainer>
    </Card>
  );
};
