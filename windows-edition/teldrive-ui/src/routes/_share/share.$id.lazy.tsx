import { SharedFileBrowser } from "@/components/shared-file-browser";
import type { SetValue } from "@/types";
import { $api } from "@/utils/api";
import { createLazyFileRoute } from "@tanstack/react-router";
import { Button, Input } from "@tw-material/react";
import { useCallback, useState } from "react";
import { Controller, useForm } from "react-hook-form";
import PasswordIcon from "~icons/carbon/password";
import HidePasswordIcon from "~icons/mdi/eye-off-outline";
import ShowPasswordIcon from "~icons/mdi/eye-outline";

export const Route = createLazyFileRoute("/_share/share/$id")({
  component: Component,
});

function Component() {
  const { id } = Route.useParams();
  const { data: file } = $api.useSuspenseQuery("get", "/shares/{id}", {
    params: {
      path: {
        id,
      },
    },
  });


  const [unlocked, setUnlocked] = useState(!file.protected);

  if (!unlocked) {
    return <ShareAccess id={id} setUnlocked={setUnlocked} />;
  }

  return !file.protected || unlocked ? <SharedFileBrowser /> : null;
}

interface ShareAccessProps {
  id: string;
  setUnlocked: SetValue<boolean>;
}

function ShareAccess({ id, setUnlocked }: ShareAccessProps) {
  const [showPassword, setShowPassword] = useState(false);

  const { control, handleSubmit, setError } = useForm({
    defaultValues: {
      password: "",
    },
  });

  const togglePassword = () => setShowPassword((prev) => !prev);

  const unLockMutation = $api.useMutation("post", "/shares/{id}/unlock", {
    onSuccess: () => {
      setUnlocked(true);
    },
    onError: () => {
      setError("password", { message: "Contraseña incorrecta" });
    },
  });

  const onSubmit = useCallback(async ({ password }: { password: string }) => {
    unLockMutation
      .mutateAsync({ params: { path: { id } }, body: { password } })
      .catch(() => {});
  }, [id, unLockMutation]);

  return (
    <form
      onSubmit={handleSubmit(onSubmit)}
      className="m-auto flex rounded-large max-w-md flex-col justify-center items-center bg-surface gap-6 p-6"
    >
      <div className="size-14 bg-secondary-container flex items-center justify-center rounded">
        <PasswordIcon className="size-8 text-on-secondary-container" />
      </div>
      <h1 className="font-medium">Este enlace está protegido con contraseña</h1>

      <Controller
        name="password"
        control={control}
        rules={{ required: true }}
        render={({ field, fieldState: { error } }) => (
          <Input
            size="lg"
            placeholder="Introduce la contraseña"
            aria-label="Contraseña del enlace"
            className="max-w-xs"
            variant="bordered"
            isInvalid={!!error}
            errorMessage={error?.message}
            {...field}
            aria-autocomplete="none"
            autoComplete="off"
            autoSave="off"
            type={showPassword ? "text" : "password"}
            endContent={
              <Button isIconOnly className="size-8 min-w-8" variant="text" onPress={togglePassword}>
                {showPassword ? <HidePasswordIcon /> : <ShowPasswordIcon />}
              </Button>
            }
          />
        )}
      />
      <Button
        isLoading={unLockMutation.isPending}
        fullWidth
        type="submit"
        variant="filledTonal"
        className="max-w-xs text-inherit"
      >
        Abrir enlace
      </Button>
    </form>
  );
}
