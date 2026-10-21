import { Button, Label, Switch } from "@heroui/react";
import { createFileRoute } from "@tanstack/react-router";
import { SettingsPageHeader, SettingsRow, SettingsSection } from "@/components/settings-layout";
import { APP_LOCALES, useAppLocale } from "@/lib/locale";
import { useEmbeddedTracks } from "@/lib/player-settings";
import { useTheme } from "@/lib/theme";
import MoonIcon from "~icons/gravity-ui/moon";
import SunIcon from "~icons/gravity-ui/sun";

export const Route = createFileRoute("/_settings/settings/appearance")({
  component: AppearanceSettings,
});

function AppearanceSettings() {
  const { theme, setTheme } = useTheme();
  const { locale, setLocale, t } = useAppLocale();
  const [embeddedTracks, setEmbeddedTracks] = useEmbeddedTracks();
  return (
    <div className="space-y-6">
      <SettingsPageHeader title={t("appearance.title")} description={t("appearance.description")} />
      <SettingsSection
        title={t("appearance.themeTitle")}
        description={t("appearance.themeDescription")}
      >
        <SettingsRow label={t("appearance.themeLabel")} description={t("appearance.themeHint")}>
          <div className="grid grid-cols-3 gap-2">
            <Button
              variant={theme === "system" ? "primary" : "secondary"}
              onPress={() => setTheme("system")}
            >
              {t("appearance.system")}
            </Button>
            <Button
              variant={theme === "light" ? "primary" : "secondary"}
              onPress={() => setTheme("light")}
            >
              <SunIcon className="size-4" />
              {t("appearance.light")}
            </Button>
            <Button
              variant={theme === "dark" ? "primary" : "secondary"}
              onPress={() => setTheme("dark")}
            >
              <MoonIcon className="size-4" />
              {t("appearance.dark")}
            </Button>
          </div>
        </SettingsRow>
      </SettingsSection>
      <SettingsSection
        title={t("appearance.languageTitle")}
        description={t("appearance.languageDescription")}
      >
        <SettingsRow
          label={t("appearance.languageLabel")}
          description={t("appearance.languageHint")}
        >
          <div className="grid grid-cols-2 gap-2">
            {APP_LOCALES.map((entry) => (
              <Button
                key={entry.code}
                variant={locale === entry.code ? "primary" : "secondary"}
                onPress={() => setLocale(entry.code)}
              >
                {entry.nativeLabel}
              </Button>
            ))}
          </div>
        </SettingsRow>
      </SettingsSection>
      <SettingsSection
        title={t("appearance.playerTitle")}
        description={t("appearance.playerDescription")}
      >
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
      </SettingsSection>
    </div>
  );
}
