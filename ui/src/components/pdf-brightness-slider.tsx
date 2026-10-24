import { Button, Slider } from "@heroui/react";
import { updateDocumentSettings, useDocumentSettings } from "@/lib/document-settings";
import { useAppLocale } from "@/lib/locale";

export function PdfBrightnessSlider() {
  const { t } = useAppLocale();
  const { pdfBrightness } = useDocumentSettings();
  return (
    <div className="space-y-3">
      <Slider
        aria-label={t("pdf.brightnessAria")}
        value={pdfBrightness}
        minValue={50}
        maxValue={150}
        step={5}
        onChange={(value) => updateDocumentSettings({ pdfBrightness: Number(value) })}
      >
        <div className="mb-2 flex justify-between text-xs">
          <span>{t("pdf.brightness")}</span>
          <span className="tabular-nums text-muted">{pdfBrightness}%</span>
        </div>
        <Slider.Track>
          <Slider.Fill />
          <Slider.Thumb />
        </Slider.Track>
      </Slider>
      <Button
        size="sm"
        variant="ghost"
        isDisabled={pdfBrightness === 100}
        onPress={() => updateDocumentSettings({ pdfBrightness: 100 })}
      >
        {t("documents.resetBrightness")}
      </Button>
    </div>
  );
}
