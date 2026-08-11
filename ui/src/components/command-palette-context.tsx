import { createContext, type ReactNode, useContext, useEffect, useMemo, useState } from "react";

type CommandPaletteContextValue = {
  isOpen: boolean;
  open: () => void;
  close: () => void;
  toggle: () => void;
};

const CommandPaletteContext = createContext<CommandPaletteContextValue | null>(null);

export function CommandPaletteProvider({ children }: { children: ReactNode }) {
  const [open, setOpen] = useState(false);

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "k") {
        event.preventDefault();
        setOpen((current) => !current);
      }
      if (event.key === "Escape") setOpen(false);
    };
    document.addEventListener("keydown", onKeyDown);
    return () => document.removeEventListener("keydown", onKeyDown);
  }, []);

  const value = useMemo(
    () => ({
      isOpen: open,
      open: () => setOpen(true),
      close: () => setOpen(false),
      toggle: () => setOpen((current) => !current),
    }),
    [open],
  );

  return <CommandPaletteContext.Provider value={value}>{children}</CommandPaletteContext.Provider>;
}

export function useCommandPalette() {
  const value = useContext(CommandPaletteContext);
  if (!value) throw new Error("useCommandPalette must be used inside CommandPaletteProvider");
  return value;
}
