import type { Rendition } from "epubjs";
import { memo, useState } from "react";
import { ReactReader } from "react-reader";

const EpubPreview = ({ assetUrl }: { assetUrl: string }) => {
  const [location, setLocation] = useState<string>();

  const onLocationChange = (cfiStr: string) => setLocation(cfiStr);

  const fixEpub = (rendition: Rendition) => {
    const spineGet = rendition.book.spine.get.bind(rendition.book.spine);
    rendition.book.spine.get = (target) => {
      let targetStr = typeof target === "string" ? target : "";
      let t = spineGet(target);
      while (t == null && targetStr.startsWith("../")) {
        targetStr = targetStr.substring(3);
        t = spineGet(targetStr);
      }
      return t;
    };
  };

  return (
    <ReactReader
      url={assetUrl}
      loadingView={<p className="p-6 text-center">Cargando libro…</p>}
      errorView={<p className="p-6 text-center">No se pudo abrir el libro. Descárgalo para abrirlo con otra aplicación.</p>}
      getRendition={(rendition) => fixEpub(rendition)}
      location={location as string}
      locationChanged={onLocationChange}
      epubInitOptions={{ openAs: "epub" }}
      epubOptions={{ flow: "scrolled", allowPopups: true }}
    />
  );
};

export default memo(EpubPreview);
