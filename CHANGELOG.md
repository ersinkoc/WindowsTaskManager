# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.4.1] - 2026-05-18

### Fixed
- **WebView2 Scroll** — Added `overflow-y: auto` and `overscroll-behavior: contain` to html/body for proper scroll support in embedded WebView2 browser

## [0.4.0] - 2026-05-17

### Added
- **GitHub Actions CI/CD** — Automated testing, building, and security audit
- **ESLint Configuration** — TypeScript-aware linting with recommended rules
- **PWA Support** — manifest.json and SVG favicon for installable app
- **Web Vitals Monitoring** — LCP, CLS, FCP, INP tracking via web-vitals
- **Global Error Handler** — window.onerror and unhandledrejection handlers
- **React ErrorBoundary** — Graceful error handling across all pages
- **Loading Skeleton Components** — TableSkeleton, ChartSkeleton, DetailSkeleton
- **Test Infrastructure** — TestWrapper with QueryClient, MemoryRouter, ThemeProvider
- **README.md** — Comprehensive installation and usage guide
- **API.md** — Complete API documentation for all endpoints

### Fixed
- **postcss XSS vulnerability** — Updated to 8.5.14 (GHSA-qx2v-qp2m-jg93)
- **Test suite** — All 21 tests now passing with proper context providers
- **Mobile Responsiveness** — Sidebar hidden on mobile (lg breakpoint)

### Changed
- **package.json** — Added typecheck, lint scripts
- **overview-page** — Improved loading states with ChartSkeleton, DetailSkeleton

## [0.3.1] - 2026-05-17

### Added
- **Ports Page Redesign** — New protocol filtering (All/TCP/UDP)
- **Dead Topbar Buttons Removed** — Cleaner UI
- **Scroll Fix** — Better scroll behavior on long lists

## [0.3.0] - 2026-05-10

### Added
- **Sparklines** — Per-process CPU/Memory trend visualization
- **AI Preview** — AI advisor suggestions shown inline
- **Keyboard Navigation** — j/k to navigate, Enter to select, Escape to close
- **Fixed Sidebar** — Narrower name column, better info density
- **Process Details Modal** — Port bindings and connection details
- **Ports Page** — Protocol filtering (All/TCP/UDP), listening/active states

### Changed
- **UI Polish** — Improved skeleton loaders and loading states
- **Performance** — Optimized re-renders with useMemo/useCallback

## [0.2.0] - 2026-05-08

### Added
- **Overview Page** — Per-core CPU, per-interface network, memory details
- **Anomaly Detection Engine** — CPU spike, memory leak, orphan, port conflict detectors
- **Alert System** — Real-time anomaly alerts with severity levels
- **Rules Engine** — User-configurable threshold-based rules
- **SSE Streaming** — Real-time metrics updates without polling

### Changed
- **New Architecture** — Separated collectors, engine, controller layers
- **Security Headers** — X-Frame-Options, CSRF token validation

## [0.1.0] - 2026-05-05

### Added
- Initial release
- Basic process listing and killing
- System metrics (CPU, Memory, Network)
- Local loopback-only web UI
- Native Windows desktop window via WebView2
