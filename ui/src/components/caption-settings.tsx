import {
  Button,
  ColorArea,
  ColorField,
  ColorPicker,
  ColorSlider,
  ColorSwatch,
  Fieldset,
  Label,
  ListBox,
  Select,
  Slider,
} from "@heroui/react";
import { DEFAULT_CAPTION_SETTINGS, I18n } from "@vplayer/react";
import { useMemo, useState } from "react";
import { toast } from "sonner";
import type { CaptionPreferences } from "@/api/types";
import { useAppLocale } from "@/lib/locale";
import { usePreferences, useUpdatePreferences } from "@/lib/preferences";
import { SettingsRow, SettingsSection } from "./settings-layout";

const fontStacks = {
  sans: "system-ui, sans-serif",
  serif: "Georgia, serif",
  monospace: "ui-monospace, monospace",
};
const numericSettings = [
  ["fontScale", "captionFontScale", 50, 200, 5],
  ["textOpacity", "captionTextOpacity", 0, 1, 0.05],
  ["backgroundOpacity", "captionBackgroundOpacity", 0, 1, 0.05],
  ["position", "captionPosition", -20, 30, 1],
  ["lineHeight", "captionLineHeight", 1, 2, 0.05],
  ["delay", "captionDelay", -10, 10, 0.1],
] as const;
const captionKeys = Object.keys(DEFAULT_CAPTION_SETTINGS) as (keyof CaptionPreferences)[];

export function CaptionSettings() {
  const { locale, t } = useAppLocale();
  const i18n = useMemo(() => new I18n(locale), [locale]);
  const labels = i18n.getPlayerLabels();
  const saved = usePreferences()?.captions ?? DEFAULT_CAPTION_SETTINGS;
  const [draft, setDraft] = useState<CaptionPreferences>(() => ({ ...saved }));
  const { mutateAsync, isPending } = useUpdatePreferences();
  const changedFields = captionKeys.filter((key) => draft[key] !== saved[key]);
  const changed = changedFields.length > 0;
  const edge =
    draft.edgeStyle === "outline"
      ? `-1px -1px 0 ${draft.edgeColor}, 1px -1px 0 ${draft.edgeColor}, -1px 1px 0 ${draft.edgeColor}, 1px 1px 0 ${draft.edgeColor}`
      : draft.edgeStyle === "shadow"
        ? `0 2px 3px ${draft.edgeColor}`
        : "none";

  async function save() {
    try {
      const patch = Object.fromEntries(
        changedFields.map((key) => [key, draft[key]]),
      ) as Partial<CaptionPreferences>;
      const result = await mutateAsync({ captions: patch });
      setDraft(result.captions);
      toast.success(labels.captionSaved);
    } catch {
      /* The shared preferences mutation displays the error and rolls back. */
    }
  }

  return (
    <SettingsSection
      title={labels.captionAppearance}
      description={t("player.captionsDescription")}
      actions={
        <div className="flex flex-wrap gap-2">
          <Button
            size="sm"
            variant="secondary"
            isDisabled={isPending}
            onPress={() => setDraft({ ...DEFAULT_CAPTION_SETTINGS })}
          >
            {labels.captionReset}
          </Button>
          <Button
            size="sm"
            variant="primary"
            isDisabled={isPending || !changed}
            onPress={() => void save()}
          >
            {labels.captionSave}
          </Button>
        </div>
      }
    >
      <div className="border-b border-border p-5">
        <figure
          aria-label={labels.captionPreview}
          className="flex min-h-24 items-center justify-center overflow-hidden rounded-xl bg-black p-5"
        >
          <span
            data-caption-preview
            className="rounded px-3 py-2 text-center"
            style={{
              fontFamily: fontStacks[draft.fontFamily],
              fontSize: `${((draft.fontSize === "small" ? 0.8 : draft.fontSize === "large" ? 1.3 : 1) * draft.fontScale) / 100}rem`,
              color: `color-mix(in srgb, ${draft.textColor} ${draft.textOpacity * 100}%, transparent)`,
              backgroundColor: `color-mix(in srgb, ${draft.backgroundColor} ${draft.backgroundOpacity * 100}%, transparent)`,
              lineHeight: draft.lineHeight,
              textShadow: edge,
            }}
          >
            {labels.captionPreview}
          </span>
        </figure>
      </div>
      <Fieldset disabled={isPending} className="min-w-0 gap-0">
        <SettingsRow label={labels.captionFontFamily}>
          <CaptionSelect
            label={labels.captionFontFamily}
            isDisabled={isPending}
            value={draft.fontFamily}
            onChange={(fontFamily) => setDraft({ ...draft, fontFamily })}
            choices={[
              ["sans", i18n.t("captionFontSans")],
              ["serif", i18n.t("captionFontSerif")],
              ["monospace", i18n.t("captionFontMono")],
            ]}
          />
        </SettingsRow>
        <SettingsRow label={labels.captionSize}>
          <CaptionSelect
            label={labels.captionSize}
            isDisabled={isPending}
            value={draft.fontSize}
            onChange={(fontSize) => setDraft({ ...draft, fontSize })}
            choices={[
              ["small", labels.captionSmall],
              ["medium", labels.captionMedium],
              ["large", labels.captionLarge],
            ]}
          />
        </SettingsRow>
        {(
          [
            ["textColor", labels.captionTextColor],
            ["backgroundColor", labels.captionBackgroundColor],
            ["edgeColor", labels.captionEdgeColor],
          ] as const
        ).map(([key, label]) => (
          <SettingsRow key={key} label={label}>
            <ColorPicker
              value={draft[key]}
              onChange={(color) =>
                setDraft((previous) => ({
                  ...previous,
                  [key]: color.toString("hex").toLowerCase(),
                }))
              }
            >
              <ColorPicker.Trigger aria-label={label} isDisabled={isPending}>
                <ColorSwatch size="lg" />
                <Label className="font-mono text-xs uppercase">{draft[key]}</Label>
              </ColorPicker.Trigger>
              <ColorPicker.Popover>
                <ColorArea
                  aria-label={label}
                  className="max-w-full"
                  colorSpace="hsb"
                  xChannel="saturation"
                  yChannel="brightness"
                  isDisabled={isPending}
                >
                  <ColorArea.Thumb />
                </ColorArea>
                <ColorSlider
                  channel="hue"
                  colorSpace="hsb"
                  className="gap-1 px-1"
                  isDisabled={isPending}
                >
                  <ColorSlider.Track>
                    <ColorSlider.Thumb />
                  </ColorSlider.Track>
                </ColorSlider>
                <ColorField aria-label={label} isDisabled={isPending}>
                  <ColorField.Group variant="secondary">
                    <ColorField.Prefix>
                      <ColorSwatch size="xs" />
                    </ColorField.Prefix>
                    <ColorField.Input />
                  </ColorField.Group>
                </ColorField>
              </ColorPicker.Popover>
            </ColorPicker>
          </SettingsRow>
        ))}
        <SettingsRow label={labels.captionEdgeStyle}>
          <CaptionSelect
            label={labels.captionEdgeStyle}
            isDisabled={isPending}
            value={draft.edgeStyle}
            onChange={(edgeStyle) => setDraft({ ...draft, edgeStyle })}
            choices={[
              ["none", labels.captionEdgeNone],
              ["shadow", labels.captionEdgeShadow],
              ["outline", labels.captionEdgeOutline],
            ]}
          />
        </SettingsRow>
        {numericSettings.map(([key, label, min, max, step]) => (
          <SettingsRow key={key} label={i18n.t(label)}>
            <Slider
              isDisabled={isPending}
              aria-label={i18n.t(label)}
              minValue={min}
              maxValue={max}
              step={step}
              value={draft[key]}
              onChange={(value) => setDraft((previous) => ({ ...previous, [key]: Number(value) }))}
            >
              <div className="mb-2 flex justify-end text-xs tabular-nums text-muted">
                <output>
                  {key.endsWith("Opacity")
                    ? `${Math.round(draft[key] * 100)}%`
                    : key === "fontScale"
                      ? `${draft[key]}%`
                      : key === "delay"
                        ? `${draft[key].toFixed(1)}s`
                        : key === "position"
                          ? `${draft[key]}vh`
                          : `${draft[key].toFixed(2)}×`}
                </output>
              </div>
              <Slider.Track>
                <Slider.Fill />
                <Slider.Thumb />
              </Slider.Track>
            </Slider>
          </SettingsRow>
        ))}
      </Fieldset>
    </SettingsSection>
  );
}

function CaptionSelect<T extends string>({
  label,
  value,
  choices,
  isDisabled,
  onChange,
}: {
  label: string;
  value: T;
  choices: readonly (readonly [T, string])[];
  isDisabled: boolean;
  onChange: (value: T) => void;
}) {
  return (
    <Select
      aria-label={label}
      selectedKey={value}
      isDisabled={isDisabled}
      onSelectionChange={(key) => {
        if (key !== null) onChange(String(key) as T);
      }}
    >
      <Select.Trigger aria-label={label}>
        <Select.Value />
        <Select.Indicator />
      </Select.Trigger>
      <Select.Popover>
        <ListBox>
          {choices.map(([key, text]) => (
            <ListBox.Item key={key} id={key} textValue={text}>
              {text}
            </ListBox.Item>
          ))}
        </ListBox>
      </Select.Popover>
    </Select>
  );
}
