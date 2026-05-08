import { create } from "zustand";

interface UIState {
  sidebarOpen: boolean;
  sidebarCollapsed: boolean;
  streamConnected: boolean;
  networkBannerDismissed: boolean;
  setSidebarOpen: (open: boolean) => void;
  setSidebarCollapsed: (collapsed: boolean) => void;
  setStreamConnected: (connected: boolean) => void;
  dismissNetworkBanner: () => void;
  resetNetworkBanner: () => void;
}

export const useUIStore = create<UIState>((set) => ({
  sidebarOpen: true,
  sidebarCollapsed: false,
  streamConnected: false,
  networkBannerDismissed: false,
  setSidebarOpen: (open) => set({ sidebarOpen: open }),
  setSidebarCollapsed: (collapsed) => set({ sidebarCollapsed: collapsed }),
  setStreamConnected: (connected) => set({ streamConnected: connected }),
  dismissNetworkBanner: () => set({ networkBannerDismissed: true }),
  resetNetworkBanner: () => set({ networkBannerDismissed: false }),
}));
