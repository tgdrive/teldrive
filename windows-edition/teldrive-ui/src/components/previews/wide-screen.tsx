import { memo } from "react";
import { Button } from "@tw-material/react";
import IconIcRoundFullscreen from "~icons/ic/round-fullscreen";
import IconIcRoundFullscreenExit from "~icons/ic/round-fullscreen-exit";
import clsx from "clsx";
import { useToggle } from "usehooks-ts";

interface WideScreenProps {
  children: React.ReactNode;
}
export const WideScreen = memo(({ children }: WideScreenProps) => {
  const [fullscreen, toggle] = useToggle(false);
  return (
    <div className="relative w-full md:w-4/5 overflow-hidden h-[calc(100%-2rem)] mx-auto">
      <div className={clsx("size-full", fullscreen ? "fixed inset-0 z-50" : "relative")}>
        <Button
          isIconOnly
          className="absolute bottom-2 right-5 z-[100]"
          variant="filled"
          onPress={toggle}
        >
          {fullscreen ? (
            <IconIcRoundFullscreenExit className="pointer-events-none" />
          ) : (
            <IconIcRoundFullscreen className="pointer-events-none" />
          )}
        </Button>
        {children}
      </div>
    </div>
  );
});
