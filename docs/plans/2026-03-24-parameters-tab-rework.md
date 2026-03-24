# Plan: Parameters Tab (TR-181 Data Tree) Rework

## Problems

1. **No URL-based navigation** — navigating deeper into the data tree doesn't change the URL. Refreshing the page always returns to `Device.`. Browser back/forward buttons don't work.

2. **No sorting** — objects and parameters are displayed in whatever order the device returns them. Should be sorted lexicographically, with numeric instance indices sorted as integers (1, 2, 10 — not 1, 10, 2).

3. **Instance objects are merged** — opening e.g. `Device.Bridging.Bridge.{i}.Port.{i}.Stats.` shows a flat list mixing all instances together. Each instance's sub-objects are not separated.

4. **No per-instance drill-down** — multi-instance objects (e.g. `Device.Bridging.Bridge.1.Port.10.`) don't show their child objects as clickable buttons to go deeper. Currently the template path `{i}` leads to ALL instances at once, mixing sub-objects.

## Current Architecture

- **URL**: `/devices/usp/{deviceID}/discovery` — never changes during navigation
- **Route**: `[...id].js` catch-all, extracts `id[0]` (deviceID) and `id[1]` (section)
- **Component**: `devices-discovery.js` (1330 lines) — holds all state in React, uses `updateDeviceParameters(path)` to fetch and display a single level
- **USP flow**: `GetSupportedDM(obj_paths=[path], first_level_only=true)` → gets object/param metadata → `Get(param_paths=[...])` → gets values
- **Navigation**: clicking arrow calls `updateDeviceParameters()` with new path; back button strips path segments. All in-memory, no URL involvement.
- **Instance handling**: `{i}` in supported_obj_path → replaced with `*` for queries → results grouped by resolved instance path (e.g. `Device.WiFi.Radio.1.`, `Device.WiFi.Radio.2.`)

## Design

### URL Structure

Current: `/devices/usp/{deviceID}/discovery`
New: `/devices/usp/{deviceID}/discovery/Device/Bridging/Bridge/1/Port/2/Stats`

The path after `discovery` encodes the TR-181 object path with dots replaced by URL path segments. The conversion is:
- `Device.Bridging.Bridge.1.Port.2.Stats.` → `/Device/Bridging/Bridge/1/Port/2/Stats`
- `/Device/Bridging/Bridge/1/Port/2/Stats` → `Device.Bridging.Bridge.1.Port.2.Stats.`

The `[...id].js` catch-all route already captures all segments. `router.query.id` for path `/devices/usp/MY_DEVICE/discovery/Device/Bridging` gives `["MY_DEVICE", "discovery", "Device", "Bridging"]`. Segments from index 2 onward form the TR-181 path.

### Per-Instance Object Display

When viewing `Device.Bridging.Bridge.*. `:
1. Query `GetSupportedDM(Device.Bridging.Bridge.*.)`  → returns template: `Device.Bridging.Bridge.{i}.` with child objects like `Port.{i}.`, `VLAN.{i}.`, etc., and params like `Enable`, `Alias`
2. Query `Get(Device.Bridging.Bridge.*.Enable, Device.Bridging.Bridge.*.Alias, ...)` → returns resolved values per instance
3. **Display each instance separately** (e.g. `Device.Bridging.Bridge.1.`, `Device.Bridging.Bridge.2.`) as its own section with:
   - Its parameter values
   - Its child **objects** as clickable drill-down buttons (e.g. `Port.` →, `VLAN.` →)
   - Each child object button navigates to that specific instance's child (e.g. clicking `Port.` under `Bridge.1.` navigates to `Device.Bridging.Bridge.1.Port.*.`)

This is the key change: child objects of a multi-instance parent are shown **per instance**, not merged across all instances.

### Sorting

- **Objects**: sorted lexicographically by name
- **Parameters**: sorted lexicographically by name
- **Instance numbers**: sorted as integers (1, 2, 3, ..., 10, 11 — not 1, 10, 11, 2, 3)

---

## Tasks

### Task 1: Extract TR-181 path from URL and sync with router

**File**: `frontend/src/sections/devices/usp/devices-discovery.js`

**Changes**:
1. Derive the current TR-181 path from `router.query.id` segments (index 2+). If no segments after `discovery`, default to `Device.`.
   ```js
   const pathSegments = router.query.id?.slice(2) || [];
   const currentPath = pathSegments.length > 0
     ? pathSegments.join('.') + '.'
     : 'Device.';
   ```

2. Replace the `useEffect(initialize)` that always starts with `Device.` — instead, call `updateDeviceParameters(currentPath)` using the URL-derived path.

3. Create a `navigateTo(tr181Path)` helper that converts a TR-181 path to a URL path and calls `router.push()`:
   ```js
   const navigateTo = (tr181Path) => {
     // "Device.Bridging.Bridge.1." → "/Device/Bridging/Bridge/1"
     const urlPath = tr181Path.replace(/\.$/, '').replaceAll('.', '/');
     router.push(
       `/devices/usp/${deviceID}/discovery/${urlPath}`,
       undefined,
       { shallow: true }
     );
   };
   ```
   Use `shallow: true` so Next.js doesn't re-run `getServerSideProps` (not used here anyway) but does update the URL.

4. Replace all `updateDeviceParameters(path)` navigation calls with `navigateTo(path)`. The actual data fetching is triggered by a `useEffect` watching `currentPath`:
   ```js
   useEffect(() => {
     updateDeviceParameters(currentPath);
   }, [currentPath]);
   ```

5. Update the back button logic: instead of computing parent path and calling `updateDeviceParameters`, call `router.back()` (or `navigateTo(parentPath)` for explicit control).

**Verification**: Navigate to `Device.WiFi.Radio.`, refresh page → should show same view. Browser back → should go to previous object.

### Task 2: Sort objects, parameters, and instance numbers

**File**: `frontend/src/sections/devices/usp/devices-discovery.js`

**Changes**:
1. In `showParameters()`, after getting `a.supported_objs`, sort them:
   ```js
   const sortedObjs = [...a.supported_objs].sort((a, b) => {
     const nameA = a.supported_obj_path.split('.').slice(-2, -1)[0];
     const nameB = b.supported_obj_path.split('.').slice(-2, -1)[0];
     return nameA.localeCompare(nameB);
   });
   ```

2. In `ShowParamsWithValues`, sort `supported_params` before rendering:
   ```js
   const sortedParams = [...(x.supported_params || [])].sort((a, b) =>
     a.param_name.localeCompare(b.param_name)
   );
   ```

3. Sort instance keys as integers when iterating `Object.keys(deviceParametersValue)`:
   ```js
   .filter(paramKey => instancePattern.test(paramKey))
   .sort((a, b) => {
     // Extract instance numbers and compare as integers
     const numsA = a.match(/\d+/g).map(Number);
     const numsB = b.match(/\d+/g).map(Number);
     for (let i = 0; i < Math.min(numsA.length, numsB.length); i++) {
       if (numsA[i] !== numsB[i]) return numsA[i] - numsB[i];
     }
     return numsA.length - numsB.length;
   })
   ```

4. Sort `supported_commands` alphabetically by `command_name` before rendering.

**Verification**: Open `Device.` — objects should be alphabetical. Open a multi-instance object — instances sorted 1, 2, 3, ..., 10, 11 (not 1, 10, 11, 2).

### Task 3: Show child objects per-instance with drill-down buttons

**File**: `frontend/src/sections/devices/usp/devices-discovery.js`

This is the core change. Currently, `GetSupportedDM` returns `supported_objs` which includes child object templates like `{i}.Port.{i}.` — but these are rendered as flat merged entries. Instead, each instance should show its own child object list.

**Changes**:

1. When `updateDeviceParameters` processes a multi-instance response (path contains `*`), separate the `supported_objs` into:
   - **The queried object itself** (index 0) — has the params/commands
   - **Child objects** (index 1+) — these are the sub-objects like `Port.{i}.`, `VLAN.{i}.`, `Stats.`

2. Store child objects in state alongside `deviceParameters`:
   ```js
   const [childObjects, setChildObjects] = useState([]);
   ```
   Extract from `supported_objs.slice(1)` — these are the child object templates.

3. In the instance rendering section (`ShowParamsWithValues` when `{i}` is detected), after rendering each instance's parameters, render its child objects as clickable buttons:
   ```js
   // For instance "Device.Bridging.Bridge.1."
   // Show child objects: Port., VLAN., Stats.
   {childObjects.map(childObj => {
     // Extract relative child name from template
     // e.g., "Device.Bridging.Bridge.{i}.Port.{i}." → "Port."
     const childName = getRelativeChildName(childObj.supported_obj_path, parentTemplate);
     // Build concrete path: "Device.Bridging.Bridge.1.Port.*."
     const concretePath = instancePath + childName.replace('{i}.', '*.');
     return (
       <ListItem key={childName}>
         <ListItemText primary={childName} />
         <IconButton onClick={() => navigateTo(concretePath)}>
           <ArrowRightIcon />
         </IconButton>
       </ListItem>
     );
   })}
   ```

4. Extract a helper `getRelativeChildName(childTemplatePath, parentTemplatePath)`:
   ```js
   // "Device.Bridging.Bridge.{i}.Port.{i}." relative to "Device.Bridging.Bridge.{i}."
   // → "Port.{i}."
   // Then for display: "Port."  (strip {i} for cleaner UI, or keep it to show it's multi-instance)
   ```

5. **Only show first-level children** — filter `supported_objs` to those that are direct children of the queried object (one level deeper), not grandchildren. Check by counting path segments relative to parent.

**Verification**: Navigate to `Device.Bridging.Bridge.*.` — should show `Bridge.1.` and `Bridge.2.` separately. Under each, child objects `Port.`, `VLAN.`, `Stats.` appear as buttons. Clicking `Port.` under `Bridge.1.` navigates to `Device.Bridging.Bridge.1.Port.*.` and the URL updates accordingly.

### Task 4: Handle URL paths with concrete instance numbers

**File**: `frontend/src/sections/devices/usp/devices-discovery.js`

When the URL contains concrete instance numbers (e.g. `.../Bridge/1/Port/2/Stats`), the component must convert these to the correct USP query paths.

**Changes**:

1. In the `currentPath` derivation, detect instance numbers and convert to `*` for the USP query, but keep the concrete numbers for display:
   ```js
   // URL path: "Device.Bridging.Bridge.1.Port.2.Stats."
   // USP query: "Device.Bridging.Bridge.1.Port.2.Stats." (concrete — queries this specific instance)
   // For GetSupportedDM: replace instance numbers with {i} to get template
   // For Get: use the concrete path
   ```

2. When `currentPath` ends with a concrete path (no `*`), send `GetSupportedDM` with the template version (replace numbers with `{i}` for schema) but `Get` with the concrete path. This shows only the specific instance's data.

3. Alternatively (simpler): when the URL has a concrete instance path like `Device.Bridging.Bridge.1.Port.2.Stats.`, query `GetSupportedDM(Device.Bridging.Bridge.*.Port.*.Stats.)` for the schema, and `Get(Device.Bridging.Bridge.1.Port.2.Stats.*)` for values of only that instance. Filter the display to show only the matching instance.

4. The `navigateTo` function should produce concrete paths when drilling into a specific instance:
   - Clicking `Stats.` under `Device.Bridging.Bridge.1.Port.2.` → navigateTo(`Device.Bridging.Bridge.1.Port.2.Stats.`) → URL becomes `.../Bridge/1/Port/2/Stats`
   - For multi-instance children: `Port.` under `Bridge.1.` → navigateTo(`Device.Bridging.Bridge.1.Port.*.`) → URL becomes `.../Bridge/1/Port` (no number = show all instances)

**Verification**: Navigate to `Device.Bridging.Bridge.1.Port.2.Stats.` directly via URL → shows only that instance's stats. Back button returns to previous view.

### Task 5: Clean up dead code and test end-to-end

**File**: `frontend/src/sections/devices/usp/devices-discovery.js`

**Changes**:
1. Remove all commented-out code blocks (lines 475-544 `initialize` old code, lines 580-600 `getDeviceParameterInstances`, lines 616-722 commented functions).
2. Remove `console.log` debugging statements throughout the file.
3. Ensure `navigateTo` is used consistently — no direct `updateDeviceParameters` calls for navigation.
4. Verify all user interactions work:
   - Click object → URL updates, view changes, data loads
   - Click back arrow → URL updates, view goes up one level
   - Browser back/forward → correct view shown
   - Refresh at any depth → same view restored
   - Add/delete instance → view refreshes correctly at current path
   - Edit parameter → value updates in place
   - Execute command → dialog works, response shown
   - Instance numbers sorted as integers
   - Objects and parameters sorted alphabetically

**Verification**: Full manual walkthrough of the parameter tree with URL checking at each step. Build the frontend Docker image to verify no build errors.
