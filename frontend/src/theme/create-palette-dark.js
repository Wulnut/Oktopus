import { alpha } from '@mui/material/styles';
import { error, indigo, info, neutral, success, warning, graphics } from './colors';

const getDarkColorScheme = () => {
  return JSON.stringify({
    "buttons": "#3d8b9e",
    "sidebar_end": "#1a2332",
    "sidebar_initial": "#0E1320",
    "tables": "#1A2332",
    "words_outside_sidebar": "#EDF2F7",
    "connected_mtps_color": "#3d8b9e"
  });
};

export function createPaletteDark() {
  let colors = getDarkColorScheme();

  let neutralColors = neutral(colors);

  return {
    action: {
      active: '#A0AEC0',
      disabled: alpha('#EDF2F7', 0.38),
      disabledBackground: alpha('#EDF2F7', 0.12),
      focus: alpha('#EDF2F7', 0.16),
      hover: alpha('#EDF2F7', 0.06),
      selected: alpha('#EDF2F7', 0.12)
    },
    background: {
      default: '#0E1320',
      paper: '#111927'
    },
    divider: '#2D3748',
    error,
    graphics,
    info,
    mode: 'dark',
    neutral: neutralColors,
    primary: indigo(colors),
    success,
    text: {
      primary: '#EDF2F7',
      secondary: '#A0AEC0',
      disabled: alpha('#EDF2F7', 0.38)
    },
    warning
  };
}
