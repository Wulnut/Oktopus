# Oktopus Frontend

## Tech Stack

- **Next.js 15** with file-based routing
- **React 19**
- **Material UI 6** (MUI) for components and theming
- **Socket.IO** client for real-time updates
- **ApexCharts** for data visualization
- **Formik/Yup** for form handling and validation

## Directory Structure

```
src/
  pages/          File-based routes (devices, firmware, scripts, tenants, etc.)
  sections/       Heavy page-specific components (devices/usp, firmware, scripts, etc.)
  components/     Shared reusable components
  contexts/       React context providers
  theme/          MUI theme configuration (light and dark palettes)
  layouts/        Dashboard layout with side nav and top nav
  guards/         Route protection (auth guard)
  hooks/          Custom React hooks
  hocs/           Higher-order components
  utils/          Utility functions
```

## Key Contexts

| Context | File | Purpose |
|---|---|---|
| Auth | `auth-context.js` | Authentication state, login/logout, JWT management |
| Tenant | `tenant-context.js` | Active tenant slug, API prefix, SuperAdmin tenant switching |
| Settings | `settings-context.js` | Theme mode (dark/light), persisted to localStorage |
| Backend | `backend-context.js` | API client with tenant-scoped base URL |
| Socket.IO | `socketio-context.js` | Real-time event subscriptions |
| Error | `error-context.js` | Global error/alert notifications |

## Theme System

The app supports dark and light themes:

- `theme/create-palette.js` -- light mode colors
- `theme/create-palette-dark.js` -- dark mode colors
- `theme/index.js` -- creates the MUI theme based on current mode
- `contexts/settings-context.js` -- `SettingsContext` manages the toggle, persisted to localStorage

The default theme is dark. Users toggle via the top nav.

## Adding a New Page

1. Create a file in `src/pages/` (e.g., `src/pages/my-page.js`).
2. Export a default component. Use `Component.getLayout` for the dashboard layout:
   ```js
   import { Layout as DashboardLayout } from 'src/layouts/dashboard/layout';
   const Page = () => { /* ... */ };
   Page.getLayout = (page) => <DashboardLayout>{page}</DashboardLayout>;
   export default Page;
   ```
3. Add a navigation entry in `src/layouts/dashboard/config.js`.
4. Create section components in `src/sections/my-page/` for complex UI.
5. Use `useTenant()` from `tenant-context.js` to get `apiPrefix` for API calls.

## Build

All builds must use Docker:

```bash
sg docker -c "cd deploy/compose && docker compose -f docker-compose.yaml -f docker-compose.dev.yaml build frontend"
```

For development with hot-reload, use `./run_debug.sh` from `deploy/compose/`.
