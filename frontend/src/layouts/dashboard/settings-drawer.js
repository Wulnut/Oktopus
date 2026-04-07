import { Box, Drawer, FormControlLabel, Stack, Switch, Typography } from '@mui/material';
import { useSettings } from 'src/contexts/settings-context';

export const SettingsDrawer = ({ open, onClose }) => {
  const { themeMode, setThemeMode } = useSettings();

  return (
    <Drawer anchor="right" open={open} onClose={onClose}>
      <Box sx={{ p: 3, width: 280 }}>
        <Typography variant="h6" sx={{ mb: 3 }}>Settings</Typography>
        <Stack spacing={2}>
          <FormControlLabel
            control={
              <Switch
                checked={themeMode === 'dark'}
                onChange={(e) => setThemeMode(e.target.checked ? 'dark' : 'light')}
              />
            }
            label="Dark Mode"
          />
        </Stack>
      </Box>
    </Drawer>
  );
};
