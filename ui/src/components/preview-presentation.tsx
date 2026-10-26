import { Button } from "@heroui/react";
import {
  createContext,
  type ReactNode,
  useCallback,
  useContext,
  useEffect,
  useRef,
  useState,
} from "react";
import { useAppLocale } from "@/lib/locale";
import RestoreIcon from "~icons/gravity-ui/chevrons-collapse-up-right";
import ExpandIcon from "~icons/gravity-ui/chevrons-expand-up-right";

const IDLE_MS = 2500;
const interactive =
  'button,a,input,textarea,select,video,audio,[role="button"],[role="slider"],[contenteditable="true"],[data-preview-chrome]';

function usePresentationState() {
  const root = useRef<HTMLDivElement>(null);
  const [expanded, setExpanded] = useState(false);
  const [visible, setVisible] = useState(true);
  const expandedRef = useRef(false);
  const visibleRef = useRef(true);
  const timer = useRef<ReturnType<typeof setTimeout>>(undefined);
  const touch = useRef<{ x: number; y: number; visible: boolean }>(undefined);

  const clearTimer = useCallback(() => clearTimeout(timer.current), []);
  const show = useCallback((value: boolean) => {
    visibleRef.current = value;
    setVisible(value);
  }, []);

  const hasOverlay = useCallback(() => {
    const element = root.current;
    if (!element) return false;
    if (element.querySelector('[aria-expanded="true"]')) return true;
    const active = document.activeElement;
    const dialog = active?.closest('[role="dialog"],[role="menu"],[role="listbox"]');
    return Boolean(
      dialog && dialog !== element && (element.contains(dialog) || !element.contains(active)),
    );
  }, []);

  const activity = useCallback(() => {
    clearTimer();
    show(true);
    if (!expandedRef.current) return;
    const hide = () => {
      const element = root.current;
      const active = document.activeElement;
      const focusedControl =
        active instanceof HTMLElement &&
        active.closest("[data-preview-chrome]") &&
        active.matches(':focus-visible,input,textarea,select,[contenteditable="true"]');
      if (hasOverlay() || focusedControl || element?.querySelector("[data-preview-chrome]:hover")) {
        timer.current = setTimeout(hide, IDLE_MS);
        return;
      }
      show(false);
    };
    timer.current = setTimeout(hide, IDLE_MS);
  }, [clearTimer, hasOverlay, show]);

  const restore = useCallback(() => {
    if (!expandedRef.current) return false;
    expandedRef.current = false;
    clearTimer();
    setExpanded(false);
    show(true);
    return true;
  }, [clearTimer, show]);

  const toggle = useCallback(() => {
    if (restore()) return;
    expandedRef.current = true;
    setExpanded(true);
    activity();
  }, [activity, restore]);

  const tap = useCallback(
    (event: PointerEvent, wasVisible: boolean) => {
      if (!expandedRef.current || event.pointerType !== "touch") return;
      // `closest` also works for an EPUB's same-origin iframe elements, whose
      // prototypes belong to a different window.
      if ((event.target as Element | null)?.closest?.(interactive)) return;
      if (hasOverlay()) {
        activity();
        return;
      }
      if (wasVisible) {
        clearTimer();
        show(false);
      } else activity();
    },
    [activity, clearTimer, hasOverlay, show],
  );

  const pointerDown = useCallback((event: PointerEvent) => {
    touch.current = { x: event.clientX, y: event.clientY, visible: visibleRef.current };
  }, []);
  const pointerMove = useCallback(
    (event: PointerEvent) => {
      if (event.pointerType !== "touch") activity();
    },
    [activity],
  );
  const pointerCancel = useCallback(() => {
    touch.current = undefined;
  }, []);
  const pointerUp = useCallback(
    (event: PointerEvent) => {
      const start = touch.current;
      if (start && Math.hypot(event.clientX - start.x, event.clientY - start.y) < 10)
        tap(event, start.visible);
      touch.current = undefined;
    },
    [tap],
  );

  useEffect(() => {
    function keydown(event: KeyboardEvent) {
      if (!expandedRef.current || event.defaultPrevented) return;
      activity();
      if (event.key === "Escape" && !hasOverlay() && !document.fullscreenElement) {
        event.preventDefault();
        event.stopImmediatePropagation();
        restore();
      }
    }
    window.addEventListener("keydown", keydown, true);
    return () => {
      window.removeEventListener("keydown", keydown, true);
      clearTimer();
    };
  }, [activity, clearTimer, hasOverlay, restore]);

  return {
    expanded,
    toggle,
    restore,
    activity,
    pointerDown,
    pointerMove,
    pointerUp,
    pointerCancel,
    rootProps: {
      ref: root,
      "data-preview-shell": "",
      "data-preview-expanded": expanded,
      "data-preview-controls": visible || !expanded,
      onPointerMoveCapture: (event: React.PointerEvent) => {
        pointerMove(event.nativeEvent);
      },
      onPointerEnter: (event: React.PointerEvent) => {
        pointerMove(event.nativeEvent);
      },
      onPointerDownCapture: (event: React.PointerEvent) => {
        pointerDown(event.nativeEvent);
      },
      onPointerUpCapture: (event: React.PointerEvent) => {
        pointerUp(event.nativeEvent);
      },
      onPointerCancelCapture: pointerCancel,
      onFocusCapture: activity,
      onBlurCapture: activity,
    },
  };
}

const PresentationContext = createContext<ReturnType<typeof usePresentationState> | null>(null);

export function PreviewPresentation({ children }: { children: ReactNode }) {
  const value = usePresentationState();
  return <PresentationContext value={value}>{children}</PresentationContext>;
}

export function usePreviewPresentation() {
  const value = useContext(PresentationContext);
  if (!value) throw new Error("PreviewPresentation is required");
  return value;
}

export function PreviewExpandButton() {
  const { t } = useAppLocale();
  const { expanded, toggle } = usePreviewPresentation();
  return (
    <Button
      isIconOnly
      size="sm"
      variant="ghost"
      aria-label={t(expanded ? "preview.restore" : "preview.expand")}
      aria-pressed={expanded}
      onPress={toggle}
    >
      {expanded ? <RestoreIcon className="size-4" /> : <ExpandIcon className="size-4" />}
    </Button>
  );
}
