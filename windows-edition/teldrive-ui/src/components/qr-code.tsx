import { useEffect, useMemo, useRef, useState } from "react";
import clsx from "clsx";
import QRCodeStyling from "qr-code-styling";

import { grow } from "@/utils/classes";

const QR_SIZE = 256;

export default function QrCode({ qrCode }: { qrCode: string }) {
  const ref = useRef(null);

  const [isQrMounted, setisQrMounted] = useState(false);

  const qrStyle = useMemo(() => {
    return new QRCodeStyling({
      width: QR_SIZE,
      height: QR_SIZE,
      margin: 10,
      type: "svg",
      dotsOptions: {
        type: "rounded",
      },
      cornersSquareOptions: {
        type: "extra-rounded",
      },
      imageOptions: {
        hideBackgroundDots: true,
        imageSize: 0.4,
        margin: 2,
      },
      qrOptions: {
        errorCorrectionLevel: "M",
      },
    });
  }, []);

  useEffect(() => {
    if (ref.current && qrStyle) {
      qrStyle.append(ref.current);
      setisQrMounted(true);
    }
  }, [qrStyle]);

  useEffect(() => {
    if (!qrCode) {
      return;
    }
    if (qrStyle) {
      qrStyle.update({ data: qrCode });
    }
  }, [qrCode, qrStyle]);

  return (
    <div
      ref={ref}
      data-mounted={isQrMounted}
      className={clsx(
        grow,
        "size-full [&>svg>rect:nth-child(3)]:fill-on-surface [&>svg>rect:nth-child(2)]:fill-surface",
      )}
    />
  );
}
