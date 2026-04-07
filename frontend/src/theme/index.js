import { createTheme as createMuiTheme } from '@mui/material';
import { createPalette } from './create-palette';
import { createPaletteDark } from './create-palette-dark';
import { createComponents } from './create-components';
import { createShadows } from './create-shadows';
import { createTypography } from './create-typography';

export function createTheme(mode = 'light') {
  const palette = mode === 'dark' ? createPaletteDark() : createPalette();
  const components = createComponents({ palette });
  const shadows = createShadows(mode);
  const typography = createTypography();

  return createMuiTheme({
    breakpoints: {
      values: {
        xs: 0,
        sm: 600,
        md: 900,
        lg: 1200,
        xl: 1440
      }
    },
    components,
    palette,
    shadows,
    shape: {
      borderRadius: 8
    },
    typography
  });
}
