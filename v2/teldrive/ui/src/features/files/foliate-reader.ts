export interface ReaderPreferences {
  theme: string;
  flow: string;
  font: string;
  fontSize: number;
  lineHeight: number;
  margin: number;
  columns: number;
}

export async function openPublication({
  element,
  file,
  preferences,
  lastLocation,
  onLoad,
  onRelocate,
}: {
  element: FoliateViewElement;
  file: File;
  preferences: ReaderPreferences;
  lastLocation?: string | { fraction: number };
  onLoad: (event: CustomEvent<{ doc: Document; index: number }>) => void;
  onRelocate: (event: CustomEvent<FoliateRelocateDetail>) => void;
}) {
  const { makeBook } = await import("foliate-js/view.js");
  const book = await makeBook(file);
  book.transformTarget?.addEventListener("data", ({ detail }: CustomEvent) => {
    detail.data = Promise.resolve(detail.data).catch((error: unknown) => {
      console.error(new Error(`Failed to load ${detail.name}`, { cause: error }));
      return "";
    });
  });

  element.addEventListener("load", onLoad as EventListener);
  element.addEventListener("relocate", onRelocate as EventListener);
  await element.open(book);
  applyPublicationAppearance(element, preferences);
  await element.init({ lastLocation, showTextStart: true });
  return book;
}

export function applyPublicationAppearance(
  element: FoliateViewElement,
  preferences: ReaderPreferences,
) {
  const { theme, flow, font, fontSize, lineHeight, margin, columns } = preferences;
  element.dataset.readerTheme = theme;
  const styles = getComputedStyle(element);
  const page = styles.getPropertyValue("--reader-page").trim();
  const text = styles.getPropertyValue("--reader-foreground").trim();
  const link = styles.getPropertyValue("--reader-link").trim();
  element.renderer?.setAttribute("flow", flow);
  element.renderer?.setAttribute("gap", "6%");
  element.renderer?.setAttribute("margin", `${margin}px`);
  element.renderer?.setAttribute("max-inline-size", "720px");
  element.renderer?.setAttribute("max-block-size", "1440px");
  element.renderer?.setAttribute("max-column-count", String(columns));
  element.renderer?.setAttribute("animated", "");
  element.renderer?.setStyles?.(`
    @namespace epub "http://www.idpf.org/2007/ops";
    :root { color-scheme: ${theme === "night" ? "dark" : "light"}; }
    html {
      color: ${text};
      line-height: ${lineHeight};
      hanging-punctuation: allow-end last;
      orphans: 2;
      widows: 2;
    }
    html, body { background: ${page} !important; }
    body {
      font-family: ${readerFont(font)} !important;
      font-size: ${fontSize}% !important;
      text-rendering: optimizeLegibility;
    }
    p, li, blockquote, dd { line-height: ${lineHeight}; }
    [align="left"] { text-align: left; }
    [align="right"] { text-align: right; }
    [align="center"] { text-align: center; }
    [align="justify"] { text-align: justify; }
    h1, h2, h3, h4, h5, h6, hgroup, th { text-wrap: balance; }
    pre { white-space: pre-wrap !important; }
    img, svg, video { max-width: 100%; }
    a:any-link { color: ${link}; }
  `);
}

export function closePublication(element: FoliateViewElement) {
  element.close();
  element.book?.destroy?.();
}

function readerFont(font: string) {
  if (font === "serif") return 'Iowan Old Style, Charter, "Bitstream Charter", Georgia, serif';
  if (font === "sans") return 'Avenir Next, Avenir, "Segoe UI", sans-serif';
  return "inherit";
}
