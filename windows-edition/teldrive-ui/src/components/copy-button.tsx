import type { FC } from "react";
import { Button, type ButtonProps } from "@tw-material/react";
import { useClipboard } from "@nextui-org/use-clipboard";
import CheckLinearIcon from "~icons/ic/round-check";
import CopyLinearIcon from "~icons/mingcute/copy-2-line";

export interface CopyButtonProps extends ButtonProps {
  value?: string;
}

export const CopyButton: FC<CopyButtonProps> = ({ value, ...buttonProps }) => {
  const { copy, copied } = useClipboard();

  const handleCopy = () => {
    copy(value);
  };

  return (
    <Button
      isIconOnly
      className="z-50 border-1 before:content-[''] before:block before:z-[-1] before:absolute before:inset-0"
      variant="filledTonal"
      onPress={handleCopy}
      disableRipple
      {...buttonProps}
    >
      <CheckLinearIcon
        className="absolute size-6 opacity-0 scale-50  data-[visible=true]:opacity-100 data-[visible=true]:scale-100 transition-transform-opacity"
        data-visible={copied}
      />
      <CopyLinearIcon
        className="absolute size-6 opacity-0 scale-50 data-[visible=true]:opacity-100 data-[visible=true]:scale-100 transition-transform-opacity"
        data-visible={!copied}
      />
    </Button>
  );
};
