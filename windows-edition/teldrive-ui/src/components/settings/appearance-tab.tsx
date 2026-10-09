import { Button, Radio, RadioGroup } from "@tw-material/react";
import clsx from "clsx";
import debounce from "lodash.debounce";
import { memo, useCallback, useEffect, useState } from "react";

import { ColorPickerMenu } from "@/components/menus/color-picker";
import { defaultColorScheme, useTheme } from "@/components/theme-provider";
import { visualThemes } from "@/config/visual-themes";
import { scrollbarClasses } from "@/utils/classes";

import IcBaselineRestartAlt from "~icons/ic/baseline-restart-alt";
import IcOutlineDarkMode from "~icons/ic/outline-dark-mode";
import IcOutlineLightMode from "~icons/ic/outline-light-mode";
import IcOutlinePalette from "~icons/ic/outline-palette";
import IcOutlineSettingsBrightness from "~icons/ic/outline-settings-brightness";

const swatches = [
  "#ff8a80",
  "#ff80ab",
  "#ea80fc",
  "#b388ff",
  "#8c9eff",
  "#82b1ff",
  "#80d8ff",
  "#84ffff",
  "#a7ffeb",
  "#b9f6ca",
  "#ccff90",
  "#f4ff81",
  "#ffff8d",
  "#ffe57f",
  "#ffd180",
  "#ff9e80",
  "#d7ccc8",
  "#f5f5f5",
  "#cfd8dc",
];

export const AppearanceTab = memo(() => {
  const { colorScheme, setColorScheme, theme, setTheme, density, setDensity } = useTheme();

  // Local state for the color picker to be snappy
  const [localColor, setLocalColor] = useState(colorScheme.color);

  // Sync local color with global state when global state changes (e.g. on reset)
  useEffect(() => {
    setLocalColor(colorScheme.color);
  }, [colorScheme.color]);

  // Debounced global state update for the color picker
  const debouncedSetColorScheme = useCallback(
    debounce((color: string) => {
      setColorScheme({ color });
    }, 200),
    [setColorScheme],
  );

  const handleColorChange = useCallback(
    (newColor: string) => {
      setLocalColor(newColor);
      debouncedSetColorScheme(newColor);
    },
    [debouncedSetColorScheme],
  );

  const handleSwatchClick = useCallback(
    (newColor: string) => {
      debouncedSetColorScheme.cancel();
      setLocalColor(newColor);
      setColorScheme({ color: newColor });
    },
    [setColorScheme, debouncedSetColorScheme],
  );

  const handleReset = useCallback(() => {
    debouncedSetColorScheme.cancel();
    setColorScheme(defaultColorScheme);
    setDensity("comfortable");
  }, [setColorScheme, setDensity, debouncedSetColorScheme]);

  useEffect(() => () => debouncedSetColorScheme.cancel(), [debouncedSetColorScheme]);

  return (
    <div className={clsx("flex flex-col gap-6 p-4 h-full overflow-y-auto", scrollbarClasses)}>
      <section className="bg-surface-container-low rounded-3xl p-6 border border-outline-variant/50 space-y-5">
        <div><h2 className="text-xl font-semibold">Temas visuales</h2><p className="text-sm text-on-surface-variant mt-1">Elige una paleta. Todos los temas admiten modo claro y oscuro.</p></div>
        <div className="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-3 gap-3">
          {visualThemes.map((preset) => <button type="button" key={preset.id} aria-pressed={colorScheme.preset === preset.id} aria-label={`Tema ${preset.name}`} onClick={() => { debouncedSetColorScheme.cancel(); setColorScheme({ color: preset.color, preset: preset.id }) }} className={clsx("text-left rounded-2xl border-2 p-3 transition-colors", colorScheme.preset === preset.id ? "border-primary bg-secondary-container" : "border-outline-variant/50 hover:bg-surface-container-high")}>
            <div className="flex h-20 rounded-lg overflow-hidden mb-3 border border-outline-variant/30" aria-hidden="true" style={{ backgroundColor: `${preset.color}12` }}>
              <div className="w-1/4 p-2 space-y-2" style={{ backgroundColor: `${preset.color}18` }}><div className="h-3 rounded" style={{ backgroundColor: preset.color }} /><div className="h-2 rounded bg-on-surface/10" /><div className="h-2 rounded bg-on-surface/10" /></div>
              <div className="flex-1 p-2 space-y-2"><div className="h-3 w-3/4 rounded-full bg-on-surface/10" /><div className="h-4 rounded" style={{ backgroundColor: `${preset.color}30` }} /><div className="h-4 rounded bg-on-surface/5" /></div>
            </div>
            <p className="font-medium text-sm">{preset.name}{colorScheme.preset === preset.id ? " · Activo" : ""}</p><p className="text-xs text-on-surface-variant mt-1">{preset.description}</p>
          </button>)}
        </div>
      </section>
      <div className="bg-surface-container-low rounded-3xl p-6 border border-outline-variant/50 flex flex-col gap-6">
        <div className="flex items-start gap-4">
          <div className="p-3 rounded-2xl bg-secondary-container">
            <IcOutlineSettingsBrightness className="size-6 text-on-secondary-container" />
          </div>
          <div className="flex-1 min-w-0">
            <h3 className="text-xl font-semibold mb-1">Modo de apariencia</h3>
            <p className="text-sm text-on-surface-variant">El modo automático sigue la apariencia del sistema.</p>
          </div>
        </div>

        <RadioGroup
          value={theme}
          onValueChange={(v) => setTheme(v as any)}
          classNames={{ wrapper: "grid grid-cols-1 md:grid-cols-3 gap-4" }}
        >
          <div
            className={clsx(
              "flex items-center gap-4 p-4 rounded-2xl cursor-pointer transition-colors border-2",
              theme === "light"
                ? "bg-secondary-container border-secondary text-on-secondary-container"
                : "bg-surface-container border-transparent hover:bg-surface-container-high",
            )}
            onClick={() => setTheme("light")}
          >
            <Radio value="light" aria-label="Claro" classNames={{ wrapper: "hidden" }} />
            <IcOutlineLightMode className="size-6" />
            <span className="font-medium">Claro</span>
          </div>

          <div
            className={clsx(
              "flex items-center gap-4 p-4 rounded-2xl cursor-pointer transition-colors border-2",
              theme === "dark"
                ? "bg-secondary-container border-secondary text-on-secondary-container"
                : "bg-surface-container border-transparent hover:bg-surface-container-high",
            )}
            onClick={() => setTheme("dark")}
          >
            <Radio value="dark" aria-label="Oscuro" classNames={{ wrapper: "hidden" }} />
            <IcOutlineDarkMode className="size-6" />
            <span className="font-medium">Oscuro</span>
          </div>

          <div
            className={clsx(
              "flex items-center gap-4 p-4 rounded-2xl cursor-pointer transition-colors border-2",
              theme === "system"
                ? "bg-secondary-container border-secondary text-on-secondary-container"
                : "bg-surface-container border-transparent hover:bg-surface-container-high",
            )}
            onClick={() => setTheme("system")}
          >
            <Radio value="system" aria-label="Automático" classNames={{ wrapper: "hidden" }} />
            <IcOutlineSettingsBrightness className="size-6" />
            <span className="font-medium">Automático</span>
          </div>
        </RadioGroup>
      </div>

      <div className="bg-surface-container-low rounded-3xl p-6 border border-outline-variant/50 flex flex-col gap-6">
        <div className="flex items-start gap-4">
          <div className="p-3 rounded-2xl bg-secondary-container">
            <IcOutlinePalette className="size-6 text-on-secondary-container" />
          </div>
          <div className="flex-1 min-w-0">
            <div className="flex items-center justify-between">
              <h3 className="text-xl font-semibold">Color personalizado</h3>
              <Button
                variant="text"
                size="sm"
                className="text-on-surface-variant hover:text-on-surface"
                startContent={<IcBaselineRestartAlt className="size-4" />}
                onPress={handleReset}
              >
                Restablecer
              </Button>
            </div>
            <p className="text-sm text-on-surface-variant">
              Usa tu color favorito para crear una paleta personalizada.
            </p>
          </div>
        </div>

        <div className="flex flex-wrap gap-4 items-center pl-1">
          {swatches.map((swatchColor) => (
            <button
              type="button"
              key={swatchColor}
              style={{ backgroundColor: swatchColor }}
              onClick={() => handleSwatchClick(swatchColor)}
              className={clsx(
                "size-10 rounded-full transition-all duration-200 border-4",
                localColor === swatchColor
                  ? "border-on-surface scale-110 shadow-lg"
                  : "border-transparent hover:scale-105",
              )}
              title={swatchColor}
            />
          ))}
          <ColorPickerMenu color={localColor} setColor={handleColorChange} />
        </div>
      </div>
      <section className="bg-surface-container-low rounded-3xl p-6 border border-outline-variant/50 space-y-4">
        <div><h3 className="text-xl font-semibold">Densidad de la interfaz</h3><p className="text-sm text-on-surface-variant mt-1">Ajusta el espacio de los paneles y la navegación.</p></div>
        <RadioGroup aria-label="Densidad de la interfaz" value={density} onValueChange={(value) => setDensity(value as "comfortable" | "compact")} orientation="horizontal"><Radio value="comfortable">Cómoda</Radio><Radio value="compact">Compacta</Radio></RadioGroup>
      </section>
    </div>
  );
});
