declare module "foliate-js/view.js";

interface FoliateRelocateDetail {
  cfi?: string;
  fraction?: number;
  location?: { current?: number; total?: number };
  tocItem?: { label?: string };
}

interface FoliateViewElement extends HTMLElement {
  book?: {
    metadata?: Record<string, unknown>;
    toc?: Array<{ label: string; href: string; subitems?: unknown[] }>;
    destroy?: () => void;
  };
  renderer?: HTMLElement & {
    setStyles?: (css: string) => void;
  };
  open: (book: File | object) => Promise<void>;
  init: (options: {
    lastLocation?: string | { fraction: number };
    showTextStart?: boolean;
  }) => Promise<void>;
  close: () => void;
  goTo: (target: string | number | object) => Promise<void>;
  goToFraction: (fraction: number) => Promise<void>;
  goLeft: () => Promise<void>;
  goRight: () => Promise<void>;
}

interface HTMLElementTagNameMap {
  "foliate-view": FoliateViewElement;
}
