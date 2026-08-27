import { create } from "zustand";
import type { Selection } from "react-aria-components";
import type { FileBrowserView } from "../file-browser";

export type FileTabLocation = {
  path: string;
  parentId?: string;
};

export type FileTab = FileTabLocation & {
  id: string;
  title: string;
  query: string;
  view: FileBrowserView;
  selectedIds: string[];
  history: FileTabLocation[];
  historyIndex: number;
  pinned: boolean;
};

type PersistedTabs = {
  tabs: Omit<FileTab, "selectedIds">[];
  activeTabId: string;
  closedTabs: Omit<FileTab, "selectedIds">[];
};

type FileTabsState = {
  tabs: FileTab[];
  activeTabId: string;
  closedTabs: FileTab[];
  hydrate: (location: FileTabLocation & { query: string; view: FileBrowserView }) => void;
  activate: (id: string) => void;
  newTab: (
    location?: Partial<FileTabLocation> & {
      title?: string;
      query?: string;
      view?: FileBrowserView;
    },
  ) => string;
  duplicate: (id: string) => string | undefined;
  close: (id: string) => void;
  closeOthers: (id: string) => void;
  closeRight: (id: string) => void;
  reopenClosed: () => string | undefined;
  togglePinned: (id: string) => void;
  reorder: (fromId: string, toId: string, placement: "before" | "after") => void;
  navigate: (id: string, location: FileTabLocation, title?: string) => void;
  back: (id: string) => void;
  forward: (id: string) => void;
  syncLocation: (id: string, location: FileTabLocation) => void;
  update: (id: string, patch: Partial<Pick<FileTab, "query" | "view" | "title">>) => void;
  setSelection: (id: string, selection: Selection) => void;
};

const storageKey = "teldrive.file-tabs.v1";
const maxClosedTabs = 12;

function newId() {
  return (
    globalThis.crypto?.randomUUID?.() ?? `${Date.now()}-${Math.random().toString(36).slice(2)}`
  );
}

function titleForPath(path: string) {
  if (path === "/") return "My files";
  return path.split("/").filter(Boolean).at(-1) ?? "My files";
}

function createTab(input: Partial<FileTab> = {}): FileTab {
  const path = input.path || "/";
  const location = { path, parentId: input.parentId };
  return {
    id: input.id || newId(),
    title: input.title || titleForPath(path),
    path,
    parentId: input.parentId,
    query: input.query || "",
    view: input.view || "list",
    selectedIds: input.selectedIds || [],
    history: input.history?.length ? input.history : [location],
    historyIndex: input.historyIndex ?? 0,
    pinned: input.pinned ?? false,
  };
}

function readPersisted(): PersistedTabs | undefined {
  try {
    const raw = localStorage.getItem(storageKey);
    if (!raw) return undefined;
    const value = JSON.parse(raw) as PersistedTabs;
    if (!Array.isArray(value.tabs) || value.tabs.length === 0) return undefined;
    return value;
  } catch {
    return undefined;
  }
}

function persist(state: Pick<FileTabsState, "tabs" | "activeTabId" | "closedTabs">) {
  try {
    localStorage.setItem(
      storageKey,
      JSON.stringify({
        tabs: state.tabs.map(({ selectedIds: _selectedIds, ...tab }) => tab),
        activeTabId: state.activeTabId,
        closedTabs: state.closedTabs
          .slice(0, maxClosedTabs)
          .map(({ selectedIds: _selectedIds, ...tab }) => tab),
      } satisfies PersistedTabs),
    );
  } catch {
    // Hardened browser contexts may block storage. Tabs remain usable in memory.
  }
}

function withPersist(
  set: (value: Partial<FileTabsState> | ((state: FileTabsState) => Partial<FileTabsState>)) => void,
) {
  return (updater: (state: FileTabsState) => Partial<FileTabsState>) =>
    set((state) => {
      const patch = updater(state);
      queueMicrotask(() => {
        const current = useFileTabsStore.getState();
        persist(current);
      });
      return patch;
    });
}

const initial = createTab();

export const useFileTabsStore = create<FileTabsState>((set, get) => {
  const updateAndPersist = withPersist(set);
  return {
    tabs: [initial],
    activeTabId: initial.id,
    closedTabs: [],
    hydrate(location) {
      const persisted = readPersisted();
      if (persisted) {
        const tabs = persisted.tabs.map((tab) => createTab({ ...tab, selectedIds: [] }));
        const activeTabId = tabs.some((tab) => tab.id === persisted.activeTabId)
          ? persisted.activeTabId
          : tabs[0].id;
        set({
          tabs,
          activeTabId,
          closedTabs: (persisted.closedTabs || []).map((tab) =>
            createTab({ ...tab, selectedIds: [] }),
          ),
        });
        return;
      }
      const tab = createTab({ ...location, title: titleForPath(location.path) });
      set({ tabs: [tab], activeTabId: tab.id, closedTabs: [] });
      persist(get());
    },
    activate(id) {
      if (!get().tabs.some((tab) => tab.id === id)) return;
      updateAndPersist(() => ({ activeTabId: id }));
    },
    newTab(location = {}) {
      const tab = createTab({
        path: location.path || "/",
        parentId: location.parentId,
        title: location.title,
        query: location.query,
        view: location.view,
      });
      updateAndPersist((state) => {
        const activeIndex = state.tabs.findIndex((item) => item.id === state.activeTabId);
        const active = state.tabs[activeIndex];
        const pinnedCount = state.tabs.filter((item) => item.pinned).length;
        const insertIndex = active?.pinned ? pinnedCount : Math.max(pinnedCount, activeIndex + 1);
        const tabs = [...state.tabs];
        tabs.splice(insertIndex, 0, tab);
        return { tabs, activeTabId: tab.id };
      });
      return tab.id;
    },
    duplicate(id) {
      const source = get().tabs.find((tab) => tab.id === id);
      if (!source) return undefined;
      const copy = createTab({ ...source, id: undefined, pinned: false, selectedIds: [] });
      updateAndPersist((state) => {
        const index = state.tabs.findIndex((tab) => tab.id === id);
        const tabs = [...state.tabs];
        tabs.splice(index + 1, 0, copy);
        return { tabs, activeTabId: copy.id };
      });
      return copy.id;
    },
    close(id) {
      updateAndPersist((state) => {
        if (state.tabs.length === 1) return {};
        const index = state.tabs.findIndex((tab) => tab.id === id);
        if (index < 0 || state.tabs[index].pinned) return {};
        const closed = state.tabs[index];
        const tabs = state.tabs.filter((tab) => tab.id !== id);
        const activeTabId =
          state.activeTabId === id
            ? tabs[Math.min(index, tabs.length - 1)]?.id || tabs[0].id
            : state.activeTabId;
        return {
          tabs,
          activeTabId,
          closedTabs: [closed, ...state.closedTabs].slice(0, maxClosedTabs),
        };
      });
    },
    closeOthers(id) {
      updateAndPersist((state) => {
        const keep = state.tabs.find((tab) => tab.id === id);
        if (!keep) return {};
        const pinned = state.tabs.filter((tab) => tab.pinned && tab.id !== id);
        const closed = state.tabs.filter((tab) => !tab.pinned && tab.id !== id);
        return {
          tabs: [...pinned, keep],
          activeTabId: id,
          closedTabs: [...closed.reverse(), ...state.closedTabs].slice(0, maxClosedTabs),
        };
      });
    },
    closeRight(id) {
      updateAndPersist((state) => {
        const index = state.tabs.findIndex((tab) => tab.id === id);
        if (index < 0) return {};
        const right = state.tabs.slice(index + 1).filter((tab) => !tab.pinned);
        const ids = new Set(right.map((tab) => tab.id));
        return {
          tabs: state.tabs.filter((tab) => !ids.has(tab.id)),
          closedTabs: [...right.reverse(), ...state.closedTabs].slice(0, maxClosedTabs),
        };
      });
    },
    reopenClosed() {
      const closed = get().closedTabs[0];
      if (!closed) return undefined;
      const tab = createTab({ ...closed, id: newId(), selectedIds: [] });
      updateAndPersist((state) => {
        const activeIndex = state.tabs.findIndex((item) => item.id === state.activeTabId);
        const pinnedCount = state.tabs.filter((item) => item.pinned).length;
        const insertIndex = Math.max(pinnedCount, activeIndex + 1);
        const tabs = [...state.tabs];
        tabs.splice(insertIndex, 0, tab);
        return {
          tabs,
          activeTabId: tab.id,
          closedTabs: state.closedTabs.slice(1),
        };
      });
      return tab.id;
    },
    togglePinned(id) {
      updateAndPersist((state) => {
        const target = state.tabs.find((tab) => tab.id === id);
        if (!target) return {};
        const updated = state.tabs.map((tab) =>
          tab.id === id ? { ...tab, pinned: !tab.pinned } : tab,
        );
        return {
          tabs: [...updated.filter((tab) => tab.pinned), ...updated.filter((tab) => !tab.pinned)],
        };
      });
    },
    reorder(fromId, toId, placement) {
      if (fromId === toId) return;
      updateAndPersist((state) => {
        const from = state.tabs.findIndex((tab) => tab.id === fromId);
        const target = state.tabs.findIndex((tab) => tab.id === toId);
        if (from < 0 || target < 0 || state.tabs[from].pinned !== state.tabs[target].pinned)
          return {};
        const tabs = [...state.tabs];
        const [moved] = tabs.splice(from, 1);
        const targetAfterRemoval = tabs.findIndex((tab) => tab.id === toId);
        const insertIndex = targetAfterRemoval + (placement === "after" ? 1 : 0);
        tabs.splice(insertIndex, 0, moved);
        return { tabs };
      });
    },
    navigate(id, location, title) {
      updateAndPersist((state) => ({
        tabs: state.tabs.map((tab) => {
          if (tab.id !== id) return tab;
          const current = tab.history[tab.historyIndex];
          if (current?.path === location.path && current?.parentId === location.parentId) {
            return {
              ...tab,
              ...location,
              title: title || titleForPath(location.path),
              query: "",
              selectedIds: [],
            };
          }
          const history = [...tab.history.slice(0, tab.historyIndex + 1), location];
          return {
            ...tab,
            ...location,
            title: title || titleForPath(location.path),
            query: "",
            selectedIds: [],
            history,
            historyIndex: history.length - 1,
          };
        }),
      }));
    },
    back(id) {
      updateAndPersist((state) => ({
        tabs: state.tabs.map((tab) => {
          if (tab.id !== id || tab.historyIndex <= 0) return tab;
          const historyIndex = tab.historyIndex - 1;
          const location = tab.history[historyIndex];
          return {
            ...tab,
            ...location,
            title: titleForPath(location.path),
            query: "",
            selectedIds: [],
            historyIndex,
          };
        }),
      }));
    },
    forward(id) {
      updateAndPersist((state) => ({
        tabs: state.tabs.map((tab) => {
          if (tab.id !== id || tab.historyIndex >= tab.history.length - 1) return tab;
          const historyIndex = tab.historyIndex + 1;
          const location = tab.history[historyIndex];
          return {
            ...tab,
            ...location,
            title: titleForPath(location.path),
            query: "",
            selectedIds: [],
            historyIndex,
          };
        }),
      }));
    },
    syncLocation(id, location) {
      updateAndPersist((state) => ({
        tabs: state.tabs.map((tab) => {
          if (tab.id !== id) return tab;
          const matchingIndexes = tab.history
            .map((entry, index) => ({ entry, index }))
            .filter(
              ({ entry }) => entry.path === location.path && entry.parentId === location.parentId,
            );
          const nearest = matchingIndexes.sort(
            (a, b) => Math.abs(a.index - tab.historyIndex) - Math.abs(b.index - tab.historyIndex),
          )[0];
          if (nearest) {
            return {
              ...tab,
              ...location,
              title: titleForPath(location.path),
              query: "",
              selectedIds: [],
              historyIndex: nearest.index,
            };
          }
          const history = [...tab.history.slice(0, tab.historyIndex + 1), location];
          return {
            ...tab,
            ...location,
            title: titleForPath(location.path),
            query: "",
            selectedIds: [],
            history,
            historyIndex: history.length - 1,
          };
        }),
      }));
    },
    update(id, patch) {
      updateAndPersist((state) => ({
        tabs: state.tabs.map((tab) => (tab.id === id ? { ...tab, ...patch } : tab)),
      }));
    },
    setSelection(id, selection) {
      const selectedIds = selection === "all" ? ["*"] : Array.from(selection, String);
      set((state) => ({
        tabs: state.tabs.map((tab) => (tab.id === id ? { ...tab, selectedIds } : tab)),
      }));
    },
  };
});

export function selectionForTab(tab: FileTab, visibleIds: string[]): Selection {
  if (tab.selectedIds.length === 1 && tab.selectedIds[0] === "*") return "all";
  const visible = new Set(visibleIds);
  return new Set(tab.selectedIds.filter((id) => visible.has(id)));
}
