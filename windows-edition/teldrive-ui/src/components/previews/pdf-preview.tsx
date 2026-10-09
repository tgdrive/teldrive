import { memo } from "react";

const PDFEmbedPreview = ({ assetUrl }: { assetUrl: string }) => {
  const url = `${assetUrl}#toolbar=1&view=FitH`;
  return (
    <iframe
      title="Vista previa del PDF"
      className="relative border-none size-full bg-white rounded-lg"
      src={url}
      allowFullScreen
    />
  );
};

export default memo(PDFEmbedPreview);
