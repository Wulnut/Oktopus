# Dark/Light Theme Toggle Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a dark/light theme toggle to the Oktopus frontend, accessible via a settings drawer in the top nav, with localStorage persistence.

**Architecture:** Extend the existing MUI theme system to accept a `mode` parameter ('light' | 'dark'). Create a `SettingsContext` to manage mode with localStorage persistence. Add a settings drawer (curtain) to the top nav with a toggle switch. Fix all hardcoded colors across the app to use theme palette references.

**Tech Stack:** Next.js 15, React 19, MUI 6, localStorage

---

## Hardcoded Colors Audit

The following files contain hardcoded color values that will break in dark mode. This plan addresses all of them.

### Phase 1 — Theme Infrastructure (Tasks 1-3)
Files that define the theme system and need mode-awareness.

### Phase 2 — Layout & Navigation (Task 4)
Sidebar, top nav, logo — hardcoded white/dark colors.

### Phase 3 — Components & Pages (Task 5)
Dialogs, code blocks, JSON viewers, message bubbles — hardcoded backgrounds and text colors.

---

## File Structure

### New files

| File | Responsibility |
|------|---------------|
| `frontend/src/contexts/settings-context.js` | SettingsContext: manages theme mode, persists to localStorage |
| `frontend/src/theme/create-palette-dark.js` | Dark mode palette definition |
| `frontend/src/layouts/dashboard/settings-drawer.js` | Settings drawer (curtain) with theme toggle |

### Modified files

| File | Changes |
|------|---------|
| `frontend/src/theme/index.js` | Accept `mode` param, select palette |
| `frontend/src/theme/create-palette.js` | Rename to light palette, clean up |
| `frontend/src/theme/create-shadows.js` | Accept `mode`, lighter shadows for dark |
| `frontend/src/theme/create-components.js` | Make overrides mode-aware |
| `frontend/src/theme/colors.js` | Add dark neutral colors |
| `frontend/src/pages/_app.js` | Wrap with SettingsProvider, dynamic theme |
| `frontend/src/layouts/dashboard/top-nav.js` | Add settings gear icon button |
| `frontend/src/layouts/dashboard/side-nav.js` | Use theme palette instead of hardcoded gradients |
| `frontend/src/layouts/dashboard/side-nav-item.js` | Replace hardcoded rgba colors with theme values |
| `frontend/src/components/logo.js` | Use theme-aware fill color |
| `frontend/src/sections/devices/usp/devices-history.js` | Replace hardcoded colors with theme palette |
| `frontend/src/sections/devices/usp/devices-discovery.js` | Replace hardcoded colors with theme palette |
| `frontend/src/sections/devices/usp/devices-lcm.js` | Replace hardcoded colors with theme palette |
| `frontend/src/sections/devices/cwmp/devices-rpc.js` | Replace hardcoded colors |
| `frontend/src/sections/devices/usp/devices-rpc.js` | Replace hardcoded colors |
| `frontend/src/sections/devices/cwmp/devices-wifi.js` | Replace hardcoded colors |
| `frontend/src/pages/credentials.js` | Replace hardcoded white |
| `frontend/src/pages/devices.js` | Replace hardcoded teal background |
| `frontend/src/sections/settings/color-theme.js` | Delete (replaced by settings-context.js) |

---

### Task 1: Settings Context and Dark Palette

**Files:**
- Create: `frontend/src/contexts/settings-context.js`
- Create: `frontend/src/theme/create-palette-dark.js`

- [ ] **Step 1: Create settings-context.js**

```javascript
import { createContext, useCallback, useContext, useEffect, useMemo, useState } from 'react';

const SettingsContext = createContext({
  themeMode: 'light',
  setThemeMode: () => {},
});

export function SettingsProvider({ children }) {
  const [themeMode, setThemeModeState] = useState('light');

  useEffect(() => {
    const stored = window.localStorage.getItem('themeMode');
    if (stored === 'dark' || stored === 'light') {
      setThemeModeState(stored);
    }
  }, []);

  const setThemeMode = useCallback((mode) => {
    setThemeModeState(mode);
    window.localStorage.setItem('themeMode', mode);
  }, []);

  const value = useMemo(() => ({
    themeMode,
    setThemeMode,
  }), [themeMode, setThemeMode]);

  return (
    <SettingsContext.Provider value={value}>
      {children}
    </SettingsContext.Provider>
  );
}

export const useSettings = () => useContext(SettingsContext);
```

- [ ] **Step 2: Create create-palette-dark.js**

Dark palette with inverted neutrals, dark backgrounds, light text. Use the same brand/accent colors (success, error, warning, info, primary) but adjust neutrals and backgrounds:

```javascript
import { alpha } from '@mui/material/styles';
import { error, indigo, info, success, warning, graphics } from './colors';

export function createPaletteDark() {
  const neutral = {
    50: '#1C2536',
    100: '#1C2536',
    200: '#2F3746',
    300: '#3F4857',
    400: '#6C737F',
    500: '#9DA4AE',
    600: '#C2C9D1',
    700: '#D9DDE3',
    800: '#E5E7EB',
    900: '#F3F4F6',
  };

  const colors = JSON.stringify({
    buttons: '#c05521',
    sidebar_end: '#305a85',
    sidebar_initial: '#173033',
    tables: '#214256',
    words_outside_sidebar: '#F3F4F6',
    connected_mtps_color: '#c05521',
  });

  return {
    action: {
      active: neutral[500],
      disabled: alpha(neutral[900], 0.38),
      disabledBackground: alpha(neutral[900], 0.12),
      focus: alpha(neutral[900], 0.16),
      hover: alpha(neutral[900], 0.04),
      selected: alpha(neutral[900], 0.12),
    },
    background: {
      default: '#0E1320',
      paper: '#111927',
    },
    divider: '#2F3746',
    error,
    graphics,
    info,
    mode: 'dark',
    neutral,
    primary: indigo(colors),
    success,
    text: {
      primary: '#EDF2F7',
      secondary: '#A0AEC0',
      disabled: alpha(neutral[900], 0.38),
    },
    warning,
  };
}
```

- [ ] **Step 3: Commit**

```bash
git add frontend/src/contexts/settings-context.js frontend/src/theme/create-palette-dark.js
git commit -m "feat: add SettingsContext and dark palette definition"
```

---

### Task 2: Theme System Mode Support

**Files:**
- Modify: `frontend/src/theme/index.js`
- Modify: `frontend/src/theme/create-shadows.js`
- Modify: `frontend/src/theme/create-components.js`

- [ ] **Step 1: Update index.js to accept mode**

```javascript
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
        xl: 1440,
      },
    },
    components,
    palette,
    shadows,
    shape: {
      borderRadius: 8,
    },
    typography,
  });
}
```

- [ ] **Step 2: Update create-shadows.js for dark mode**

Read the current file. It has 25 shadow definitions using `rgba(0, 0, 0, 0.08)`. For dark mode, shadows should be stronger (darker backdrop needs more pronounced shadows) or use a different alpha. Update the function to accept `mode`:

```javascript
export function createShadows(mode = 'light') {
  const shadowColor = mode === 'dark' ? 'rgba(0, 0, 0, 0.4)' : 'rgba(0, 0, 0, 0.08)';
  // Replace all shadow definitions to use shadowColor
  // Keep the same offsets/blur, just change the rgba values
}
```

- [ ] **Step 3: Update create-components.js for dark mode**

Read the file. Key changes:
- Card boxShadow (line ~61): use palette-based shadow instead of hardcoded
- MuiFilledInput root: `borderColor` and `backgroundColor` should come from palette
- MuiOutlinedInput: hover background and notchedOutline borderColor already use palette — these should be OK
- MuiTableHead: background color should use palette.neutral

The function already receives `{ palette }` so it's mode-aware through the palette values. Most overrides reference `palette.neutral[*]` which will differ between light/dark. Review each override and replace any remaining hardcoded values.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/theme/
git commit -m "feat: make theme system mode-aware (light/dark)"
```

---

### Task 3: Wire Theme Toggle into App

**Files:**
- Modify: `frontend/src/pages/_app.js`
- Create: `frontend/src/layouts/dashboard/settings-drawer.js`
- Modify: `frontend/src/layouts/dashboard/top-nav.js`

- [ ] **Step 1: Update _app.js to use SettingsContext and dynamic theme**

Read the current `_app.js`. It currently creates theme once. Change to:
- Wrap app with `SettingsProvider` (outside `AuthProvider`)
- Inside the app component, use `useSettings()` to get `themeMode`
- Create theme with `createTheme(themeMode)` using `useMemo`

```javascript
import { SettingsProvider, useSettings } from 'src/contexts/settings-context';

// Inside the app component:
const { themeMode } = useSettings();
const theme = useMemo(() => createTheme(themeMode), [themeMode]);
```

Note: `SettingsProvider` must wrap the component that calls `useSettings()`. Either split into inner/outer components or put `SettingsProvider` at the very top.

- [ ] **Step 2: Create settings-drawer.js**

A MUI Drawer anchored right, containing a theme mode toggle:

```javascript
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
```

- [ ] **Step 3: Add gear icon to top-nav.js**

Import `Cog6ToothIcon` from heroicons and `SettingsDrawer`. Add state for drawer open/close. Add an IconButton next to the avatar:

```javascript
import Cog6ToothIcon from '@heroicons/react/24/solid/Cog6ToothIcon';
import { SettingsDrawer } from './settings-drawer';

// In component:
const [settingsOpen, setSettingsOpen] = useState(false);

// In JSX, before the Avatar:
<IconButton onClick={() => setSettingsOpen(true)}>
  <SvgIcon fontSize="small">
    <Cog6ToothIcon />
  </SvgIcon>
</IconButton>
<SettingsDrawer open={settingsOpen} onClose={() => setSettingsOpen(false)} />
```

- [ ] **Step 4: Verify build and test toggle**

Build: `sg docker -c "cd /home/maksim/projects/oktopus/deploy/compose && docker compose -f docker-compose.yaml -f docker-compose.dev.yaml build frontend 2>&1"`

- [ ] **Step 5: Commit**

```bash
git add frontend/src/pages/_app.js frontend/src/layouts/dashboard/settings-drawer.js frontend/src/layouts/dashboard/top-nav.js
git commit -m "feat: add settings drawer with dark mode toggle"
```

---

### Task 4: Fix Layout & Navigation Colors

**Files:**
- Modify: `frontend/src/layouts/dashboard/side-nav.js`
- Modify: `frontend/src/layouts/dashboard/side-nav-item.js`
- Modify: `frontend/src/components/logo.js`

- [ ] **Step 1: Fix side-nav.js**

Read the file. The sidebar uses hardcoded gradient backgrounds and white text. Replace:

- `background: linear-gradient(...)` — keep the gradient but it should work in both modes since the sidebar is always dark. The sidebar can stay dark in both modes (common pattern). But the `rgba(255, 255, 255, 0.04)` overlays and `color: 'common.white'` are fine for an always-dark sidebar.

Actually, since the sidebar is intentionally dark in both modes, the main fix is ensuring it doesn't conflict. Review and keep as-is if the sidebar stays dark, or update to use palette values if it should change with mode.

- [ ] **Step 2: Fix side-nav-item.js**

Same as sidebar — if sidebar stays dark, the item colors (white text, white hover overlays) are correct. No changes needed for the sidebar items if the sidebar remains dark in both modes.

- [ ] **Step 3: Fix logo.js**

Currently: `const fillColor = '#FFFFFF'`. The logo is used in the sidebar (dark background) so white is correct. But if it's also used elsewhere (e.g., login page), it may need the theme. Check usage and update if needed:

```javascript
import { useTheme } from '@mui/material/styles';
// In component:
const theme = useTheme();
const fillColor = theme.palette.mode === 'dark' ? '#FFFFFF' : '#FFFFFF';
// Logo on dark sidebar is always white — keep as is unless used elsewhere
```

- [ ] **Step 4: Commit**

```bash
git add frontend/src/layouts/dashboard/ frontend/src/components/logo.js
git commit -m "fix: review layout and nav colors for dark mode compatibility"
```

---

### Task 5: Fix Hardcoded Colors in Components & Pages

**Files:**
- Modify: `frontend/src/sections/devices/usp/devices-history.js`
- Modify: `frontend/src/sections/devices/usp/devices-discovery.js`
- Modify: `frontend/src/sections/devices/usp/devices-lcm.js`
- Modify: `frontend/src/sections/devices/cwmp/devices-rpc.js`
- Modify: `frontend/src/sections/devices/usp/devices-rpc.js`
- Modify: `frontend/src/sections/devices/cwmp/devices-wifi.js`
- Modify: `frontend/src/pages/credentials.js`
- Modify: `frontend/src/pages/devices.js`

This is the bulk of the work. For each file, replace hardcoded colors with theme-aware equivalents.

- [ ] **Step 1: Fix devices-history.js**

This file has the most hardcoded colors (~30 instances). Key patterns to fix:

| Pattern | Light value | Dark replacement |
|---------|------------|-----------------|
| `color: '#666'` | gray text | `color: 'text.secondary'` |
| `color: '#999'` | lighter gray | `color: 'text.disabled'` |
| `color: 'black'` | black text | `color: 'text.primary'` |
| `color: '#fff'` | white text | `color: 'common.white'` (in colored bubbles — keep) |
| `backgroundColor: '#f5f5f5'` | light bg | `bgcolor: 'action.hover'` |
| `backgroundColor: 'rgba(0,0,0,0.04)'` | hover | `bgcolor: 'action.hover'` |
| `backgroundColor: 'rgba(0,0,0,0.1)'` | hover | `bgcolor: 'action.selected'` |
| `backgroundColor: 'rgba(255,255,255,0.7)'` | overlay | `bgcolor: (theme) => alpha(theme.palette.background.paper, 0.7)` |
| `border: '1px solid #e0e0e0'` | border | `border: 1, borderColor: 'divider'` |
| JSON syntax colors | Various | Use theme-aware syntax colors (or keep — they're in colored bubbles with light bg) |

For JSON syntax highlighting colors (`#0B7500`, `#1A01CC`, `#881391`), these need both light and dark variants. Use `theme.palette.mode` check:
```javascript
const theme = useTheme();
const syntaxColors = theme.palette.mode === 'dark'
  ? { string: '#98C379', number: '#61AFEF', boolean: '#61AFEF', key: '#C678DD', null: '#ABB2BF' }
  : { string: '#0B7500', number: '#1A01CC', boolean: '#1A01CC', key: '#881391', null: '#999' };
```

- [ ] **Step 2: Fix devices-discovery.js**

Replace:
- `boxShadow: 'rgba(149, 157, 165, 0.2) 0px 0px 5px'` — use theme shadow or `(theme) => theme.shadows[2]`
- `backgroundColor: 'rgba(0,0,0,0.02)'` — use `'action.hover'`
- `backgroundColor: 'rgba(255,255,255,0.5)'` — use `(theme) => alpha(theme.palette.background.paper, 0.5)`
- `backgroundColor: '#f5f5f5'` — use `'action.hover'`
- `color: '#d32f2f'` — use `'error.main'`
- `color: 'black'` — use `'text.primary'`
- `color: '#fff'` — use `'common.white'` (in overlay contexts — keep)

- [ ] **Step 3: Fix devices-lcm.js**

Replace:
- `backgroundColor: 'rgba(0, 0, 0, 0.7)'` (modal overlay) — keep, works in both
- `backgroundColor: 'rgba(0, 0, 0, 0.02)'` — use `'action.hover'`
- `backgroundColor: '#f5f5f5'` — use `'action.hover'`
- `borderColor: 'rgba(0, 0, 0, 0.23)'` — use `'divider'`
- Orange animation colors (`rgba(255, 152, 0, ...)`) — keep, they're accent colors

- [ ] **Step 4: Fix devices-rpc.js (both USP and CWMP)**

Replace in both `frontend/src/sections/devices/usp/devices-rpc.js` and `frontend/src/sections/devices/cwmp/devices-rpc.js`:
- `backgroundColor: "rgba(48, 109, 111, 0.04)"` — use `'action.hover'`
- `color: '#fff'` (in Backdrop) — keep, Backdrop is always dark
- `color: 'black'` — use `'text.primary'`

- [ ] **Step 5: Fix devices-wifi.js (CWMP)**

Replace:
- `backgroundColor: 'rgba(255,255,255,0.5)'` — use `(theme) => alpha(theme.palette.background.paper, 0.5)`

- [ ] **Step 6: Fix pages (credentials.js, devices.js)**

- `credentials.js`: `color: '#fff'` in Backdrop — keep
- `devices.js`: `backgroundColor: "rgba(48, 109, 111, 0.04)"` — use `'action.hover'`

- [ ] **Step 7: Delete unused color-theme.js**

```bash
rm frontend/src/sections/settings/color-theme.js
```

- [ ] **Step 8: Verify build**

Build: `sg docker -c "cd /home/maksim/projects/oktopus/deploy/compose && docker compose -f docker-compose.yaml -f docker-compose.dev.yaml build frontend 2>&1"`

- [ ] **Step 9: Commit**

```bash
git add frontend/src/sections/ frontend/src/pages/ frontend/src/components/
git rm frontend/src/sections/settings/color-theme.js
git commit -m "fix: replace hardcoded colors with theme-aware values for dark mode"
```

---

### Task 6: Visual Verification

- [ ] **Step 1: Start app and test**

Restart frontend, toggle between light and dark mode. Verify:
- Overview page: stat cards, charts, CPE settings
- Devices list page
- Device detail pages (info, history, discovery, LCM, RPC)
- Tenants page
- Users page
- Settings page
- Credentials page
- Firmware page

- [ ] **Step 2: Fix any remaining contrast issues**

Check for:
- Text readability on dark backgrounds
- Border visibility
- Chart legend readability
- Dialog/modal backgrounds
- Form input backgrounds and borders

- [ ] **Step 3: Final commit**

```bash
git add -A
git commit -m "fix: dark mode visual refinements after testing"
```
