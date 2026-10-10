import { memo } from "react";

const DocPreview = ({ assetUrl }: { assetUrl: string }) => {
  const url = `https://view.officeapps.live.com/op/view.aspx?src=${encodeURIComponent(assetUrl)}`;

  return (
    <div className="flex flex-col h-full gap-3">
    <p className="text-xs text-white/70 text-center">La vista de Office usa Microsoft y requiere que el archivo sea accesible desde Internet. Para enlaces con contraseña o un servidor local, descarga el documento.</p>
    <iframe
      title="Vista previa del documento"
      className="relative border-none w-full flex-1 rounded-lg bg-white"
      src={url}
      allowFullScreen
    />
    </div>
  );
};

export default memo(DocPreview);
