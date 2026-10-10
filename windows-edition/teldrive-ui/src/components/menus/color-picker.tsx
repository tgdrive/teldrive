import { Button, Popover, PopoverContent, PopoverTrigger } from "@tw-material/react";
import { memo } from "react";
import { HexColorPicker } from "react-colorful";
import TablerColorPicker from "~icons/tabler/color-picker";

interface ColorPickerMenuProps {
  color: string;
  setColor: (color: string) => void;
}
export const ColorPickerMenu = memo(({ color, setColor }: ColorPickerMenuProps) => {
  return (
    <Popover placement="left-start" offset={16}>
      <PopoverTrigger>
        <Button
          title="Elegir color"
          variant="filledTonal"
          isIconOnly
          className="min-w-10 size-10 rounded-xl bg-secondary-container text-on-secondary-container"
        >
          <TablerColorPicker className="size-5" />
        </Button>
      </PopoverTrigger>
      <PopoverContent className="flex flex-col gap-2 p-3 bg-surface-container-high border border-outline-variant/30 rounded-[24px] shadow-2xl relative">
        <HexColorPicker className="!w-full !h-40" color={color} onChange={setColor} />
        <div className="flex items-center gap-2 px-1 pt-1">
          <div
            className="size-6 rounded-full border border-outline-variant"
            style={{ backgroundColor: color }}
          />
          <span className="text-sm font-mono font-medium text-on-surface-variant uppercase">
            {color}
          </span>
        </div>
      </PopoverContent>
    </Popover>
  );
});
