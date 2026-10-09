import {
  Button,
  Description,
  FieldError,
  Input,
  Label,
  Switch,
  TextArea,
  TextField,
} from "@heroui/react";
import { createFormHook, createFormHookContexts } from "@tanstack/react-form";
import type { ComponentProps } from "react";

const { fieldContext, formContext, useFieldContext, useFormContext } = createFormHookContexts();

function fieldErrors(errors: unknown[]) {
  return errors
    .flatMap((error) => {
      if (typeof error === "string") return [error];
      if (
        error &&
        typeof error === "object" &&
        "message" in error &&
        typeof error.message === "string"
      ) {
        return [error.message];
      }
      return [];
    })
    .filter(Boolean);
}

function useFieldPresentation() {
  const field = useFieldContext<unknown>();
  const errors = fieldErrors(field.state.meta.errors);
  const isInvalid =
    (field.state.meta.isTouched || field.form.state.submissionAttempts > 0) && errors.length > 0;

  return {
    field,
    errors,
    isInvalid,
  };
}

type CommonFieldProps = {
  label?: string;
  description?: string;
  isRequired?: boolean;
  isDisabled?: boolean;
  autoFocus?: boolean;
};

type AppTextFieldProps = CommonFieldProps &
  Omit<ComponentProps<typeof Input>, "value" | "onChange" | "onBlur" | "isDisabled">;

function AppTextField({
  label,
  description,
  isRequired,
  isDisabled,
  ...inputProps
}: AppTextFieldProps) {
  const { field, errors, isInvalid } = useFieldPresentation();
  const value = typeof field.state.value === "string" ? field.state.value : "";

  return (
    <TextField
      className="grid gap-1"
      isRequired={isRequired}
      isDisabled={isDisabled}
      isInvalid={isInvalid}
    >
      {label ? <Label>{label}</Label> : null}
      <Input
        {...inputProps}
        value={value}
        onBlur={field.handleBlur}
        onChange={(event) => field.handleChange(event.currentTarget.value)}
      />
      {description ? <Description>{description}</Description> : null}
      {isInvalid ? <FieldError>{errors.join(". ")}</FieldError> : null}
    </TextField>
  );
}

type AppTextAreaFieldProps = CommonFieldProps &
  Omit<ComponentProps<typeof TextArea>, "value" | "onChange" | "onBlur" | "isDisabled">;

function AppTextAreaField({
  label,
  description,
  isRequired,
  isDisabled,
  ...textAreaProps
}: AppTextAreaFieldProps) {
  const { field, errors, isInvalid } = useFieldPresentation();
  const value = typeof field.state.value === "string" ? field.state.value : "";

  return (
    <TextField
      className="grid gap-1"
      isRequired={isRequired}
      isDisabled={isDisabled}
      isInvalid={isInvalid}
    >
      {label ? <Label>{label}</Label> : null}
      <TextArea
        {...textAreaProps}
        value={value}
        onBlur={field.handleBlur}
        onChange={(event) => field.handleChange(event.currentTarget.value)}
      />
      {description ? <Description>{description}</Description> : null}
      {isInvalid ? <FieldError>{errors.join(". ")}</FieldError> : null}
    </TextField>
  );
}

type AppSwitchFieldProps = CommonFieldProps & {
  children?: never;
};

function AppSwitchField({
  label,
  description,
  isDisabled,
  "aria-label": ariaLabel,
}: AppSwitchFieldProps & { "aria-label"?: string }) {
  const field = useFieldContext<boolean>();

  return (
    <Switch
      aria-label={ariaLabel}
      isSelected={field.state.value}
      isDisabled={isDisabled}
      onBlur={field.handleBlur}
      onChange={field.handleChange}
    >
      <Switch.Content>
        <Switch.Control>
          <Switch.Thumb />
        </Switch.Control>
        {label ? <Label>{label}</Label> : null}
        {description ? <Description>{description}</Description> : null}
      </Switch.Content>
    </Switch>
  );
}

type SubmitButtonProps = Omit<
  ComponentProps<typeof Button>,
  "type" | "isPending" | "isDisabled"
> & {
  requireDirty?: boolean;
};

function SubmitButton({ requireDirty = false, children, ...props }: SubmitButtonProps) {
  const form = useFormContext();

  return (
    <form.Subscribe
      selector={(state) => [state.canSubmit, state.isSubmitting, state.isDirty] as const}
    >
      {([canSubmit, isSubmitting, isDirty]) => (
        <Button
          {...props}
          type="submit"
          isPending={isSubmitting}
          isDisabled={!canSubmit || isSubmitting || (requireDirty && !isDirty)}
        >
          {children}
        </Button>
      )}
    </form.Subscribe>
  );
}

export const { useAppForm, withForm } = createFormHook({
  fieldComponents: {
    TextField: AppTextField,
    TextAreaField: AppTextAreaField,
    SwitchField: AppSwitchField,
  },
  formComponents: {
    SubmitButton,
  },
  fieldContext,
  formContext,
});
