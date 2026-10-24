import { Label, Switch } from "@heroui/react";
import { createFileRoute } from "@tanstack/react-router";
import { CaptionSettings } from "@/components/caption-settings";
import { SettingsPageHeader, SettingsRow, SettingsSection } from "@/components/settings-layout";
import { useAppLocale } from "@/lib/locale";
import { useEmbeddedTracks } from "@/lib/player-settings";
import { usePreferences, useUpdatePreferences } from "@/lib/preferences";

export const Route = createFileRoute("/_settings/settings/player")({
  component: PlayerSettings,
});

function PlayerSettings() {
  const { t } = useAppLocale();
  const [embeddedTracks, setEmbeddedTracks] = useEmbeddedTracks();
  const onShares = usePreferences()?.appearance.embeddedTracksOnShares === true;
  const { mutate: updatePreferences } = useUpdatePreferences();
  return (
    <div className="space-y-6">
      <SettingsPageHeader
        title={t("settings.item.player")}
        description={t("appearance.playerDescription")}
      />
      <SettingsSection title={t("appearance.playerTitle")}>
        <SettingsRow
          label={t("appearance.embeddedTracksLabel")}
          description={t("appearance.embeddedTracksHint")}
        >
          <Switch
            aria-label={t("appearance.embeddedTracksSwitch")}
            isSelected={embeddedTracks}
            onChange={setEmbeddedTracks}
          >
            <Switch.Content>
              <Switch.Control>
                <Switch.Thumb />
              </Switch.Control>
              <Label>{t("appearance.embeddedTracksSwitch")}</Label>
            </Switch.Content>
          </Switch>
        </SettingsRow>
        <SettingsRow
          label={t("appearance.embeddedTracksSharesLabel")}
          description={t("appearance.embeddedTracksSharesHint")}
        >
          <Switch
            aria-label={t("appearance.embeddedTracksSharesSwitch")}
            isSelected={embeddedTracks && onShares}
            isDisabled={!embeddedTracks}
            onChange={(enabled) =>
              updatePreferences({ appearance: { embeddedTracksOnShares: enabled } })
            }
          >
            <Switch.Content>
              <Switch.Control>
                <Switch.Thumb />
              </Switch.Control>
              <Label>{t("appearance.embeddedTracksSharesSwitch")}</Label>
            </Switch.Content>
          </Switch>
        </SettingsRow>
      </SettingsSection>
      <CaptionSettings />
    </div>
  );
}
