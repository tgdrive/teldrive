import Editor from "@monaco-editor/react";
import { Spinner } from "@tw-material/react";
import { memo } from "react";

import useFileContent from "@/hooks/use-file-content";
import { getLanguageByFileName } from "@/utils/preview-type";

interface CodePreviewProps {
  name: string;
  assetUrl: string;
}
const CodePreview = ({ name, assetUrl }: CodePreviewProps) => {
  const { response: content, validating, error } = useFileContent(assetUrl);
  if (error) return <p role="alert" className="text-white/80 p-6 text-center">No se pudo cargar el contenido. Descarga el archivo para abrirlo.</p>;

  return (
    <>
      {validating ? <Spinner aria-label="Cargando archivo" /> : (
        <Editor
          loading={<Spinner />}
          defaultLanguage={getLanguageByFileName(name)}
          theme="vs-dark"
          height="100%"
          value={content}
          options={{ readOnly: true, minimap: { enabled: false }, wordWrap: "on", contextmenu: false }}
        />
      )}
    </>
  );
};

export default memo(CodePreview);
