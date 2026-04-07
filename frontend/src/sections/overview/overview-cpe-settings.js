import React, { useState } from 'react';
import PropTypes from 'prop-types';
import {
  Box,
  Button,
  Card,
  CardContent,
  CardHeader,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  Snackbar,
  TextField,
  Typography,
} from '@mui/material';

const getProtocolParams = (protocol, host, tenantSlug) => {
  const t = tenantSlug || 'default';
  switch (protocol) {
    case 'MQTT':
      return [
        ['Device.LocalAgent.MTP.1.Protocol', 'MQTT'],
        ['Device.LocalAgent.MTP.1.Enable', 'true'],
        ['Device.LocalAgent.MTP.1.MQTT.Reference', 'Device.MQTT.Client.1'],
        ['Device.LocalAgent.MTP.1.MQTT.ResponseTopicConfigured', `oktopus/usp/v1/${t}/agent/<endpoint_id>`],
        ['Device.MQTT.Client.1.BrokerAddress', `mqtt://${host}:1883`],
        ['Device.MQTT.Client.1.TransportProtocol', 'TCP/IP'],
        ['Device.MQTT.Client.1.Username', '<device_serial>'],
        ['Device.MQTT.Client.1.Password', '<device_password>'],
        ['Device.LocalAgent.Controller.1.EndpointID', 'oktopusController'],
        ['Device.LocalAgent.Controller.1.MTP.1.Protocol', 'MQTT'],
        ['Device.LocalAgent.Controller.1.MTP.1.MQTT.Topic', `oktopus/usp/v1/${t}/controller/<endpoint_id>`],
      ];
    case 'WebSocket':
      return [
        ['Device.LocalAgent.MTP.1.Protocol', 'WebSocket'],
        ['Device.LocalAgent.MTP.1.Enable', 'true'],
        ['Device.LocalAgent.Controller.1.EndpointID', 'oktopusController'],
        ['Device.LocalAgent.Controller.1.MTP.1.Protocol', 'WebSocket'],
        ['Device.LocalAgent.Controller.1.MTP.1.WebSocket.Host', host],
        ['Device.LocalAgent.Controller.1.MTP.1.WebSocket.Port', '8080'],
        ['Device.LocalAgent.Controller.1.MTP.1.WebSocket.Path', `/${t}/`],
      ];
    case 'STOMP':
      return [
        ['Device.LocalAgent.MTP.1.Protocol', 'STOMP'],
        ['Device.LocalAgent.MTP.1.Enable', 'true'],
        ['Device.LocalAgent.MTP.1.STOMP.Reference', 'Device.STOMP.Connection.1'],
        ['Device.LocalAgent.MTP.1.STOMP.Destination', `oktopus/usp/v1/${t}/agent/<endpoint_id>`],
        ['Device.STOMP.Connection.1.Host', host],
        ['Device.STOMP.Connection.1.Port', '61613'],
        ['Device.STOMP.Connection.1.Username', '<device_serial>'],
        ['Device.STOMP.Connection.1.Password', '<device_password>'],
        ['Device.LocalAgent.Controller.1.EndpointID', 'oktopusController'],
        ['Device.LocalAgent.Controller.1.MTP.1.Protocol', 'STOMP'],
        ['Device.LocalAgent.Controller.1.MTP.1.STOMP.Destination', `oktopus/usp/v1/${t}/controller/<endpoint_id>`],
      ];
    case 'CWMP':
      return [
        ['Device.ManagementServer.URL', `http://${host}:9292/acs/${t}/`],
        ['Device.ManagementServer.Username', '<device_serial>'],
        ['Device.ManagementServer.Password', '<device_password>'],
        ['Device.ManagementServer.PeriodicInformEnable', 'true'],
        ['Device.ManagementServer.PeriodicInformInterval', '300'],
      ];
    default:
      return [];
  }
};

const protocols = [
  { name: 'MQTT', rttKey: 'MqttRtt' },
  { name: 'WebSocket', rttKey: 'WebsocketsRtt' },
  { name: 'STOMP', rttKey: 'StompRtt' },
  { name: 'CWMP', rttKey: 'AcsRtt' },
];

export const OverviewCpeSettings = (props) => {
  const { generalInfo, tenantSlug, sx } = props;
  const [dialogOpen, setDialogOpen] = useState(false);
  const [selectedProtocol, setSelectedProtocol] = useState(null);
  const [copied, setCopied] = useState(false);

  const host = typeof window !== 'undefined' ? window.location.hostname : 'localhost';

  const handleRowClick = (protocol) => {
    setSelectedProtocol(protocol);
    setDialogOpen(true);
  };

  const handleClose = () => {
    setDialogOpen(false);
    setSelectedProtocol(null);
  };

  const getParamsText = () => {
    if (!selectedProtocol) return '';
    const params = getProtocolParams(selectedProtocol, host, tenantSlug);
    return params.map(([key, value]) => `${key} ${value}`).join('\n');
  };

  const handleCopy = () => {
    navigator.clipboard.writeText(getParamsText()).then(() => {
      setCopied(true);
    });
  };

  return (
    <>
      <Card sx={sx}>
        <CardHeader title="CPE Settings" />
        <CardContent>
          {protocols.map((proto) => {
            const rttValue = generalInfo?.[proto.rttKey];
            const isOnline = rttValue && rttValue !== '';

            return (
              <Box
                key={proto.name}
                onClick={() => handleRowClick(proto.name)}
                sx={{
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'space-between',
                  py: 1.5,
                  px: 2,
                  mb: 1,
                  borderRadius: 1,
                  cursor: 'pointer',
                  '&:hover': {
                    bgcolor: 'action.hover',
                  },
                }}
              >
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1.5 }}>
                  <Box
                    sx={{
                      width: 10,
                      height: 10,
                      borderRadius: '50%',
                      bgcolor: isOnline ? 'success.main' : 'error.main',
                      flexShrink: 0,
                    }}
                  />
                  <Typography variant="body1">{proto.name}</Typography>
                </Box>
                <Typography variant="body2" color="text.secondary">
                  {isOnline ? rttValue : 'offline'}
                </Typography>
              </Box>
            );
          })}
        </CardContent>
      </Card>

      <Dialog open={dialogOpen} onClose={handleClose} maxWidth="md" fullWidth>
        <DialogTitle>
          {selectedProtocol} -- TR-181 Parameters
        </DialogTitle>
        <DialogContent>
          <TextField
            fullWidth
            multiline
            value={getParamsText()}
            slotProps={{
              input: {
                readOnly: true,
                sx: {
                  fontFamily: 'monospace',
                  fontSize: '0.85rem',
                  lineHeight: 1.8,
                },
              },
            }}
            sx={{ mt: 1 }}
          />
        </DialogContent>
        <DialogActions>
          <Button onClick={handleCopy} variant="contained" size="small">
            Copy to Clipboard
          </Button>
          <Button onClick={handleClose} size="small">
            Close
          </Button>
        </DialogActions>
      </Dialog>

      <Snackbar
        open={copied}
        autoHideDuration={2000}
        onClose={() => setCopied(false)}
        message="Copied to clipboard"
      />
    </>
  );
};

OverviewCpeSettings.propTypes = {
  generalInfo: PropTypes.object,
  sx: PropTypes.object,
};
