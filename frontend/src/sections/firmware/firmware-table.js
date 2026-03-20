import PropTypes from 'prop-types';
import {
  Box,
  Card,
  Chip,
  IconButton,
  Skeleton,
  Stack,
  SvgIcon,
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableRow,
  Tooltip,
  Typography,
} from '@mui/material';
import PencilIcon from '@heroicons/react/24/outline/PencilIcon';
import TrashIcon from '@heroicons/react/24/outline/TrashIcon';
import ArrowUpCircleIcon from '@heroicons/react/24/outline/ArrowUpCircleIcon';
import ArrowDownCircleIcon from '@heroicons/react/24/outline/ArrowDownCircleIcon';
import { Scrollbar } from 'src/components/scrollbar';

const PHASE_CONFIG = {
  internal_testing: { label: 'Internal Testing', color: 'info' },
  release: { label: 'Release', color: 'success' },
};

const formatFileSize = (bytes) => {
  if (!bytes || bytes === 0) return '—';
  const kb = bytes / 1024;
  return `${kb.toLocaleString(undefined, { maximumFractionDigits: 1 })} KB`;
};

const truncateString = (str, maxLen) => {
  if (!str) return '—';
  if (str.length <= maxLen) return str;
  return str.slice(0, maxLen) + '...';
};

const formatDate = (dateStr) => {
  if (!dateStr) return '—';
  try {
    return new Date(dateStr).toLocaleString();
  } catch {
    return dateStr;
  }
};

export const FirmwareTable = ({ firmware = [], loading = false, onDelete, onPhaseChange, onEdit }) => {
  const skeletonRows = Array.from({ length: 4 });

  return (
    <Card>
      <Scrollbar>
        <Box sx={{ minWidth: 1000 }}>
          <Table>
            <TableHead>
              <TableRow>
                <TableCell>Name</TableCell>
                <TableCell>Vendor</TableCell>
                <TableCell>Model</TableCell>
                <TableCell>Build Version</TableCell>
                <TableCell>File Size</TableCell>
                <TableCell>Fingerprint</TableCell>
                <TableCell>Phase</TableCell>
                <TableCell>Download URL</TableCell>
                <TableCell>Created At</TableCell>
                <TableCell align="right">Actions</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {loading
                ? skeletonRows.map((_, i) => (
                    <TableRow key={i}>
                      {Array.from({ length: 10 }).map((__, j) => (
                        <TableCell key={j}>
                          <Skeleton variant="text" width="80%" />
                        </TableCell>
                      ))}
                    </TableRow>
                  ))
                : firmware.length === 0
                ? (
                    <TableRow>
                      <TableCell colSpan={10} align="center">
                        <Typography color="text.secondary" sx={{ py: 3 }}>
                          No firmware found. Upload your first firmware image.
                        </Typography>
                      </TableCell>
                    </TableRow>
                  )
                : firmware.map((fw) => {
                    const phaseConfig = PHASE_CONFIG[fw.phase] || { label: fw.phase, color: 'default' };
                    const isRelease = fw.phase === 'release';

                    return (
                      <TableRow hover key={fw.id}>
                        <TableCell>
                          <Typography variant="body2" fontWeight={500}>
                            {fw.name || '—'}
                          </Typography>
                        </TableCell>
                        <TableCell>{fw.vendor || '—'}</TableCell>
                        <TableCell>{fw.model || '—'}</TableCell>
                        <TableCell>{fw.build_version || '—'}</TableCell>
                        <TableCell>{formatFileSize(fw.file_size)}</TableCell>
                        <TableCell>
                          {fw.fingerprint ? (
                            <Tooltip title={fw.fingerprint} placement="top">
                              <Typography variant="body2" sx={{ fontFamily: 'monospace', fontSize: '0.75rem' }}>
                                {truncateString(fw.fingerprint, 12)}
                              </Typography>
                            </Tooltip>
                          ) : (
                            '—'
                          )}
                        </TableCell>
                        <TableCell>
                          <Chip
                            label={phaseConfig.label}
                            color={phaseConfig.color}
                            size="small"
                          />
                        </TableCell>
                        <TableCell>
                          {fw.download_url ? (
                            <Tooltip title={fw.download_url} placement="top">
                              <Typography
                                component="a"
                                href={fw.download_url}
                                target="_blank"
                                rel="noopener noreferrer"
                                variant="body2"
                                sx={{
                                  color: 'primary.main',
                                  textDecoration: 'none',
                                  '&:hover': { textDecoration: 'underline' },
                                  fontFamily: 'monospace',
                                  fontSize: '0.75rem',
                                }}
                              >
                                {truncateString(fw.download_url, 30)}
                              </Typography>
                            </Tooltip>
                          ) : (
                            '—'
                          )}
                        </TableCell>
                        <TableCell>
                          <Typography variant="body2" color="text.secondary">
                            {formatDate(fw.created_at)}
                          </Typography>
                        </TableCell>
                        <TableCell align="right">
                          <Stack direction="row" spacing={0.5} justifyContent="flex-end">
                            <Tooltip title="Edit" placement="top">
                              <IconButton
                                size="small"
                                onClick={() => onEdit && onEdit(fw)}
                                sx={{
                                  color: 'warning.main',
                                  '&:hover': { backgroundColor: 'warning.light' },
                                }}
                              >
                                <SvgIcon fontSize="small">
                                  <PencilIcon />
                                </SvgIcon>
                              </IconButton>
                            </Tooltip>
                            <Tooltip
                              title={isRelease ? 'Demote to Internal Testing' : 'Promote to Release'}
                              placement="top"
                            >
                              <IconButton
                                size="small"
                                onClick={() =>
                                  onPhaseChange &&
                                  onPhaseChange(fw.id, isRelease ? 'internal_testing' : 'release')
                                }
                                sx={{
                                  color: isRelease ? 'warning.main' : 'success.main',
                                  '&:hover': {
                                    backgroundColor: isRelease ? 'warning.light' : 'success.light',
                                  },
                                }}
                              >
                                <SvgIcon fontSize="small">
                                  {isRelease ? <ArrowDownCircleIcon /> : <ArrowUpCircleIcon />}
                                </SvgIcon>
                              </IconButton>
                            </Tooltip>
                            <Tooltip title="Delete firmware" placement="top">
                              <IconButton
                                size="small"
                                onClick={() => onDelete && onDelete(fw.id)}
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
                          </Stack>
                        </TableCell>
                      </TableRow>
                    );
                  })}
            </TableBody>
          </Table>
        </Box>
      </Scrollbar>
    </Card>
  );
};

FirmwareTable.propTypes = {
  firmware: PropTypes.array,
  loading: PropTypes.bool,
  onDelete: PropTypes.func,
  onEdit: PropTypes.func,
  onPhaseChange: PropTypes.func,
};
