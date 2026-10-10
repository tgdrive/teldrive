import { type CheckboxProps, Chip, VisuallyHidden, tv, useCheckbox } from "@tw-material/react";

const checkbox = tv({
  slots: {
    base: "border-outline border-1 bg-transparent select-none transition-all duration-200 cursor-pointer hover:bg-on-surface-variant/5 px-3 py-1.5 outline-none",
    content: "text-current capitalize font-medium",
  },
  variants: {
    isSelected: {
      true: {
        base: "bg-secondary-container border-secondary-container text-on-secondary-container hover:bg-secondary-container/90",
      },
    },
  },
});

interface FilterChipProps extends CheckboxProps {
  startIcon: React.ReactNode;
}

export const FilterChip = ({ startIcon, ...props }: FilterChipProps) => {
  const { children, isSelected, getBaseProps, getLabelProps, getInputProps } = useCheckbox({
    ...props,
  });

  const styles = checkbox({ isSelected });

  return (
    <label {...getBaseProps()} className="cursor-pointer">
      <VisuallyHidden>
        <input {...getInputProps()} />
      </VisuallyHidden>
      <Chip
        variant="bordered"
        classNames={{
          base: styles.base(),
          content: styles.content(),
        }}
        startContent={startIcon}
        {...getLabelProps()}
      >
        {children}
      </Chip>
    </label>
  );
};
