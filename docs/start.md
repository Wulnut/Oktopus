This report outlines the current status of your project based on the **OktopUSP** framework and details the roadmap for future enhancements based on the features observed in the provided sources.

# Project Roadmap: OktopUSP Enhancement

## 1. Project Overview
The project currently utilizes a modified version of the **OktopUSP** (USP Controller) project. Significant progress has already been made in **improving performance** and **enhancing existing features** to ensure a more robust USP (User Services Platform) environment. The next phase of development aims to bridge the gap between the current backend capabilities and a comprehensive management interface similar to the **TR369 Cloud Platform UI**.

---

## 2. Current State
*   **Foundation:** Based on the OktopUSP open-source project.
*   **Optimization:** Successful performance tuning and feature enhancements.
*   **USP Support:** Basic TR-369 protocol handling for device management.
*   **Firmware Management:** Upload, version, and deploy firmware images with vendor/model matching and phase tracking (Internal Testing → Release).
*   **Per-Device Dashboard:** Info, Network, Performance, Bridging, and Topology tabs with real-time metrics and historical charts.
*   **Offline Device Access:** Device Info tab shows cached data when the device is offline. Other tabs display an offline banner. The "Access the device" button is available for all devices on the /devices list.
*   **Scripts:** Saved sequences of USP commands (GET/SET/ADD/DELETE/OPERATE) with variables, conditions, and delays. Execution history with per-step results.
*   **Mass Actions:** Batch firmware updates and script execution across multiple devices with concurrency control, progress tracking, and cancellation.
*   **Multi-Tenancy:** Full tenant isolation with three user levels (SuperAdmin/TenantAdmin/Operator), per-tenant databases (`tenant_<slug>_general`, `tenant_<slug>_usp`), tenant-scoped NATS subjects, and tenant management UI.
*   **Dark/Light Theme:** Theme toggle persisted to localStorage, with dedicated light and dark palettes.
*   **Tenant-Scoped Container Registry:** Container images prefixed with tenant slug for namespace isolation.
*   **CPE Settings:** TR-181 parameter display on the Overview page, organized by protocol.
*   **Tenant Deletion:** Full cleanup including databases, NATS KeyValue buckets, and associated users.

---

## 3. Development Goals & Feature Requirements

Drawing from the reference material, the following three core pillars have been identified for the next development cycle:

### A. Firmware (FW) Upgrade & File Server Integration
To manage device lifecycles effectively, an **Over-the-Air (OTA) update system** is required.
*   **Management Interface:** A dedicated "Firmware Update" section allowing administrators to see firmware names, build versions, file sizes, and fingerprints.
*   **Workflow:** Support for distinct phases, specifically **Internal Testing** and **Release**.
*   **File Server:** A system to host firmware images (*.BIN files) with an upload interface and support for optional download URLs.
*   **Status Tracking:** Real-time feedback on file upload status and deployment progress (e.g., "Success", "Stop", or "Official Release").

### B. Per-Device Dashboard
A centralized "View" for individual devices is necessary to provide granular control and visibility.
*   **Control Actions:** Immediate access to remote commands such as **Reboot**, **Factory Reset**, **Restart Agent**, and **Refresh Information**.
*   **Real-time Metrics:** Gauges for **CPU Usage** and **RAM Usage**, alongside storage status (System, Used, and Free space).
*   **Categorized Data Tabs:**
    *   **Metadata:** Model, brand, chipset, and software versioning.
    *   **Network Interfaces:** Detailed IPv4/IPv6 data for Ethernet, and comprehensive Wi-Fi configuration (SSID, security, channel, transmit power) for 2.4G, 5G, and Guest networks.
    *   **Performance Tracking:** Historical line charts for average utilization of CPU, RAM, and ROM.

### C. Network Topology
Visualizing the local network structure is a priority, utilizing **TR-181 nodes** to map device relationships.
*   **Connectivity Mapping:** A "Connection" tab to display how devices are linked within the network.
*   **Device Categorization:** The ability to filter and view different device types (e.g., **Wired devices**).
*   **Link Quality:** Monitoring of **Transmission Rates** and online status for all connected sub-devices.
*   **Synchronization:** A "Synchronize" function to pull the latest topology data from the TR-181 data model via the USP agent.

---

## 4. Implementation Strategy
*   **Backend:** Leverage existing OktopUSP USP message handling to query specific TR-181 objects (e.g., `Device.WiFi.`, `Device.Ethernet.`, and `Device.DeviceInfo.`).
*   **Frontend:** Develop a UI that mirrors the clean, tabbed navigation seen in the sources, prioritizing clear status indicators (Online/Offline) and intuitive action buttons.
*   **Storage:** Implement a file server module capable of handling large firmware binaries and mapping them to specific device groups or models.

---

## 5. Implemented Features

### A. Firmware Management (Completed)
*   **MongoDB schema** (`db/firmware.go`): Firmware CRUD with vendor, model, phase, download URL fields.
*   **Upload service** (`backend/services/utils/firmware-upload/`): Node.js file upload on port 8006.
*   **REST API** (`api/firmware.go`): List, upload metadata, update, delete, phase transitions.
*   **Frontend** (`pages/firmware.js`, `sections/firmware/`): Firmware management page with upload, phase toggles, and vendor/model editing.

### B. Per-Device Dashboard (Completed)
*   **Device Info** (`devices-info.js`): Displays device metadata (Manufacturer, Model, HW/SW version). Supports firmware update, reboot, and factory reset actions. Caches info in MongoDB for offline access.
*   **Network** (`devices-network.js`): Wi-Fi and Ethernet/IP interface details.
*   **Performance** (`devices-performance.js`): Real-time CPU/RAM gauges with historical charts (ApexCharts). Metrics stored with TTL indexes.
*   **Bridging** (`devices-bridging.js`): Bridge and port management.
*   **Topology** (`devices-topology.js`): Network topology visualization (WiFi clients, hosts, Ethernet links).
*   **Offline support**: Info tab shows cached data when device is offline (no USP queries sent). Other tabs show "Device is Offline" banner. Device info is cached as raw JSON to avoid MongoDB BSON serialization issues.

### C. Network Topology (Completed)
*   **Backend** (`api/topology.go`): Queries WiFi clients, hosts, and Ethernet interfaces via USP GET.
*   **Frontend** (`devices-topology.js`): Displays connected devices with signal strength and link type.

### D. Scripts (Completed)
*   **Execution engine** (`api/scripts.go`): Sequential step execution with variable substitution, conditions, and delays.
*   **Frontend** (`pages/scripts/`): Script editor with variable definitions, step builder, and execution history.

### E. Mass Actions (Completed)
*   **Batch firmware updates** (`api/mass_actions.go`): Concurrent execution with semaphore-based concurrency control. Auto-filters devices by firmware vendor/model.
*   **Batch script execution**: Same concurrent pattern with per-device execution tracking.
*   **Job tracking**: Progress bars, per-device status, cancellation support, auto-refresh while running.
*   **Frontend** (`pages/mass-actions/firmware.js`, `pages/mass-actions/scripts.js`): Firmware and script mass action pages with device selector and job history.

### F. Multi-Tenancy (Completed)
*   **Tenant management** (`api/tenant.go`, `db/tenant.go`): CRUD for tenants with slug-based routing, status (active/disabled), auth policies, and CA certificates.
*   **User roles**: Three-tier model -- SuperAdmin (level 0), TenantAdmin (level 1), Operator (level 2). JWT claims carry tenant_id, tenant_slug, and level.
*   **Database isolation** (`db.TenantDB`): Per-tenant databases (`tenant_<slug>_general`, `tenant_<slug>_usp`). API handlers use `a.tenantDB(r)` for all tenant-scoped data access.
*   **Middleware** (`middleware.go`): `AuthMiddleware` validates JWT. `TenantMiddleware` enforces tenant-scoping and verifies tenant existence.
*   **NATS isolation**: Subjects include tenant slug (e.g., `adapter.usp.v1.<slug>.devices.<action>`). Per-tenant KeyValue buckets for device auth.
*   **Adapter service**: Tenant-aware -- filters devices by `tenantid`, extracts tenant slug from NATS subjects.
*   **Container registry**: `container-upload` service prefixes images with tenant slug.
*   **Frontend** (`pages/tenants/`, `contexts/tenant-context.js`): Tenant management page, tenant context for API routing, SuperAdmin tenant switcher.
*   **Tenant deletion**: Full cleanup of databases, NATS buckets, and associated users.

### G. Dark/Light Theme (Completed)
*   **Theme toggle** (`contexts/settings-context.js`): SettingsContext with localStorage persistence.
*   **Palettes** (`theme/create-palette.js`, `theme/create-palette-dark.js`): Separate light and dark color schemes.
*   **Integration**: Theme mode applied via MUI's `createTheme` in `theme/index.js`.
