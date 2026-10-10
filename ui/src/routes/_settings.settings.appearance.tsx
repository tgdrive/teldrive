import { Button } from "@heroui/react";
import { createFileRoute } from "@tanstack/react-router";
import { SettingsPageHeader, SettingsRow, SettingsSection } from "@/components/settings-layout";
import { useTheme } from "@/lib/theme";
import MoonIcon from "~icons/gravity-ui/moon";
import SunIcon from "~icons/gravity-ui/sun";

export const Route = createFileRoute("/_settings/settings/appearance")({
  component: AppearanceSettings,
});

function AppearanceSettings() {
  const { theme, setTheme } = useTheme();
  return (
    <div className="space-y-6">
      <SettingsPageHeader
        title="Appearance"
        description="Control how Teldrive looks in this browser."
      />
      <SettingsSection
        title="Color theme"
        description="The choice is stored locally and applies immediately."
      >
        <SettingsRow label="Theme" description="Follow your system, or choose light or dark.">
          <div className="grid grid-cols-3 gap-2">
            <Button
              variant={theme === "system" ? "primary" : "secondary"}
              onPress={() => setTheme("system")}
            >
              System
            </Button>
            <Button
              variant={theme === "light" ? "primary" : "secondary"}
              onPress={() => setTheme("light")}
            >
              <SunIcon className="size-4" />
              Light
            </Button>
            <Button
              variant={theme === "dark" ? "primary" : "secondary"}
              onPress={() => setTheme("dark")}
            >
              <MoonIcon className="size-4" />
              Dark
            </Button>
          </div>
        </SettingsRow>
      </SettingsSection>
    </div>
  );
}
