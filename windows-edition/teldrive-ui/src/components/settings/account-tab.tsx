import type { UserSession } from "@/types";
import { useQueryClient, useSuspenseQueries } from "@tanstack/react-query";
import {
  Button,
  Input,
  Modal,
  ModalBody,
  ModalContent,
  ModalFooter,
  ModalHeader,
  Radio,
  RadioGroup,
  Textarea,
} from "@tw-material/react";
import clsx from "clsx";
import { memo, useCallback, useState } from "react";
import IcRoundContentCopy from "~icons/ic/round-content-copy";
import IcRoundRemoveCircleOutline from "~icons/ic/round-remove-circle-outline";

import { Controller, useForm } from "react-hook-form";
import toast from "react-hot-toast";

import { $api } from "@/utils/api";
import { scrollbarClasses } from "@/utils/classes";
import { copyDataToClipboard } from "@/utils/common";
import { spanishError } from "@/utils/locale";

import type { components } from "@/lib/api";
import { NetworkError } from "@/utils/fetch-throw";
import IcRoundSecurity from "~icons/ic/round-security";
import AddIcon from "~icons/material-symbols/add-circle";
import DeleteIcon from "~icons/material-symbols/delete";
import MaterialSymbolsSmartToy from "~icons/material-symbols/smart-toy";
import SyncIcon from "~icons/material-symbols/sync";
import MaterialSymbolsTv from "~icons/material-symbols/tv";

const validateBots = (value?: string) => {
  if (value) {
    const regexPattern = /^\d{10}:[A-Za-z\d_-]{35}$/gm;
    return regexPattern.test(value) || "Formato de token incorrecto";
  }
  return false;
};

const formatDate = (date: string) => {
  const d = new Date(date);
  return d.toLocaleDateString("es-CL", {
    month: "short",
    day: "numeric",
    year: "numeric",
  });
};

const Session = memo(
  ({ appName, location, createdAt, valid, hash, current }: UserSession) => {
    const deleteSession = $api.useMutation("delete", "/users/sessions/{id}", {
      onSettled: () => {
        queryClient.invalidateQueries({
          queryKey: $api.queryOptions("get", "/users/sessions").queryKey,
        });
      },
    });
    const queryClient = useQueryClient();

    return (
      <div className="bg-surface-container rounded-2xl p-4 border border-outline-variant/30 transition-all duration-300 relative group">
        <div className="flex items-start justify-between mb-3">
          <div className="flex items-center gap-2">
            <div
              className={clsx(
                "w-2 h-2 rounded-full",
                valid ? "bg-green-500" : "bg-red-500",
              )}
            />
            <p className="font-medium text-base">{appName || "Desconocido"}</p>
            {current && (
              <span className="text-xs px-2 py-0.5 rounded-full bg-secondary-container text-on-secondary-container font-medium">
                Actual
              </span>
            )}
          </div>
          {(!current || !valid) && (
            <Button
              isIconOnly
              variant="text"
              size="sm"
              className="opacity-0 group-hover:opacity-100 transition-opacity data-[hover=true]:text-error"
              onPress={() =>
                deleteSession.mutateAsync({ params: { path: { id: hash } } })
              }
            >
              <DeleteIcon className="size-5" />
            </Button>
          )}
        </div>

        <div className="space-y-1.5">
          <div className="flex items-center gap-2 text-sm text-on-surface-variant">
            <span>Creación</span>
            <span className="font-medium text-on-surface">•</span>
            <span className="font-medium text-on-surface">
              {formatDate(createdAt)}
            </span>
          </div>
          {location && (
            <div className="flex items-center gap-2 text-sm text-on-surface-variant">
              <span>Ubicación</span>
              <span className="font-medium text-on-surface">•</span>
              <span className="font-medium text-on-surface">{location}</span>
            </div>
          )}
        </div>
      </div>
    );
  },
);

const ChannelCreateDialog = ({ handleClose }: { handleClose: () => void }) => {
  const queryClient = useQueryClient();
  const createChannel = $api.useMutation("post", "/users/channels", {
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["get", "/users/channels"] });
      toast.success("Canal añadido");
    },
  });

  const [channel, setChannel] = useState("");

  const onCreate = useCallback(
    (e: React.FormEvent<HTMLDivElement>) => {
      e.preventDefault();
      createChannel
        .mutateAsync({
          body: {
            channelName: channel,
          },
        })
        .then(() => handleClose());
    },
    [channel],
  );

  return (
    <>
      <ModalHeader className="flex flex-col gap-1">Crear canal</ModalHeader>
      <ModalBody as="form" id="add-channel" onSubmit={onCreate}>
        <Input
          size="lg"
          variant="bordered"
          classNames={{
            inputWrapper: "border-primary border-large",
          }}
          placeholder="Nombre del canal"
          autoFocus
          value={channel}
          onValueChange={setChannel}
        />
      </ModalBody>
      <ModalFooter>
        <Button className="font-normal" variant="text" onPress={handleClose}>
          Cerrar
        </Button>
        <Button
          type="submit"
          form="add-channel"
          className="font-normal"
          variant="filledTonal"
          isDisabled={createChannel.isPending || !channel}
          isLoading={createChannel.isPending}
        >
          {createChannel.isPending ? "Creando…" : "Crear"}
        </Button>
      </ModalFooter>
    </>
  );
};

const BotRemoveDialog = ({
  handleClose,
  onRemove,
}: {
  handleClose: () => void;
  onRemove: () => void;
}) => {
  return (
    <>
      <ModalHeader className="flex flex-col gap-1">Eliminar todos los bots</ModalHeader>
      <ModalBody>
        <p className="text-lg font-medium mt-2">
          ¿Quieres eliminar todos los bots?
        </p>
        <p className="text-sm text-on-surface-variant mt-1">
          Esta acción no se puede deshacer.
        </p>
      </ModalBody>
      <ModalFooter>
        <Button className="font-normal" variant="text" onPress={handleClose}>
          Cancelar
        </Button>
        <Button
          variant="filledTonal"
          className="font-normal bg-error-container text-on-error-container data-[hover=true]:bg-error-container/80 transition-colors"
          onPress={onRemove}
        >
          Eliminar todos
        </Button>
      </ModalFooter>
    </>
  );
};

const ChannelDeleteDialog = ({
  channelId,
  handleClose,
}: {
  channelId: number;
  handleClose: () => void;
}) => {
  const queryClient = useQueryClient();

  const deleteChannel = $api.useMutation("delete", "/users/channels/{id}", {
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["get", "/users/channels"] });
      toast.success("Canal eliminado");
    },
  });

  const onDelete = useCallback(() => {
    deleteChannel
      .mutateAsync({
        params: {
          path: {
            id: String(channelId),
          },
        },
      })
      .then(() => handleClose());
  }, [channelId]);
  return (
    <>
      <ModalHeader className="flex flex-col gap-1">Eliminar canal</ModalHeader>
      <ModalBody>
        <p className="text-lg font-medium mt-2">
          ¿Quieres eliminar este canal?
        </p>
      </ModalBody>
      <ModalFooter>
        <Button className="font-normal" variant="text" onPress={handleClose}>
          No
        </Button>
        <Button
          variant="filledTonal"
          classNames={{
            base: "font-normal",
          }}
          isLoading={deleteChannel.isPending}
          onPress={onDelete}
        >
          Sí
        </Button>
      </ModalFooter>
    </>
  );
};

interface ChannelOperationProps {
  open: boolean;
  handleClose: () => void;
  operation: "add" | "delete";
  channelId: number;
}

interface BotOperationProps {
  open: boolean;
  handleClose: () => void;
  onRemove: () => void;
}

const BotOperationModal = memo(
  ({ open, handleClose, onRemove }: BotOperationProps) => {
    return (
      <Modal
        isOpen={open}
        size="md"
        classNames={{
          wrapper: "overflow-hidden",
          base: "bg-surface w-full shadow-none",
        }}
        placement="center"
        onClose={handleClose}
        hideCloseButton
      >
        <ModalContent>
          <BotRemoveDialog handleClose={handleClose} onRemove={onRemove} />
        </ModalContent>
      </Modal>
    );
  },
);

const ChannelOperationModal = memo(
  ({ open, handleClose, operation, channelId }: ChannelOperationProps) => {
    const renderOperation = () => {
      switch (operation) {
        case "add":
          return <ChannelCreateDialog handleClose={handleClose} />;
        case "delete":
          return (
            <ChannelDeleteDialog
              channelId={channelId}
              handleClose={handleClose}
            />
          );
        default:
          return null;
      }
    };
    return (
      <Modal
        isOpen={open}
        size="md"
        classNames={{
          wrapper: "overflow-hidden",
          base: "bg-surface w-full shadow-none",
        }}
        placement="center"
        onClose={handleClose}
        hideCloseButton
      >
        <ModalContent>{renderOperation}</ModalContent>
      </Modal>
    );
  },
);

export const AccountTab = memo(() => {
  const { control, handleSubmit } = useForm<{ tokens: string }>({
    defaultValues: { tokens: "" },
  });

  const [{ data: userConfig }, { data: sessions }, { data: channelData }] =
    useSuspenseQueries({
      queries: [
        $api.queryOptions("get", "/users/config"),
        $api.queryOptions("get", "/users/sessions"),
        $api.queryOptions("get", "/users/channels"),
      ],
    });

  const removeBots = $api.useMutation("delete", "/users/bots", {
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["get", "/users/config"] });
      toast.success("Se eliminaron todos los bots");
      setBotOpen(false);
    },
    onError: () => {
      toast.error("No se pudieron eliminar los bots");
    },
  });

  const handleRemoveBots = useCallback(() => {
    removeBots.mutate({});
  }, []);

  const syncChannels = $api.useMutation("patch", "/users/channels/sync", {
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["get", "/users/channels"] });
      toast.success("Canales sincronizados");
    },
    onError: async (error) => {
      if (error instanceof NetworkError) {
        const errorData =
          (await error.data?.json()) as components["schemas"]["Error"];
        toast.error(
          spanishError(errorData.message),
        );
      } else {
        toast.error("No se pudo sincronizar. Se produjo un error desconocido.");
      }
    },
  });

  const queryClient = useQueryClient();

  const copyTokens = useCallback(() => {
    if (userConfig && userConfig.bots.length > 0) {
      copyDataToClipboard(userConfig.bots).then(() => {
        toast.success("Tokens copiados");
      });
    }
  }, [userConfig?.bots]);

  const botAddition = $api.useMutation("post", "/users/bots", {
    onSuccess: () => {
      toast.success("Bots añadidos");
      queryClient.invalidateQueries({ queryKey: ["get", "/users/config"] });
    },
    onError: async (error) => {
      if (error instanceof NetworkError) {
        const errorData =
          (await error.data?.json()) as components["schemas"]["Error"];
        toast.error(spanishError(errorData.message));
      }
    },
  });

  const updateChannel = $api.useMutation("patch", "/users/channels", {
    onSuccess: () => {
      toast.success("Canal predeterminado actualizado");
      queryClient.invalidateQueries({ queryKey: ["get", "/users/config"] });
    },
    onError: async (error) => {
      if (error instanceof NetworkError) {
        const errorData =
          (await error.data?.json()) as components["schemas"]["Error"];
        toast.error(
          spanishError(errorData.message),
        );
      } else {
        toast.error(
          "No se pudo actualizar el canal predeterminado. Se produjo un error desconocido.",
        );
      }
    },
  });

  const onSubmit = useCallback(
    async ({ tokens }: { tokens: string }) => {
      botAddition.mutateAsync({
        body: {
          bots: tokens.trim().split("\n"),
        },
      });
    },
    [botAddition],
  );

  const handleSetDefaultChannel = useCallback(
    (channelId: number) => {
      const channel = channelData?.find((c) => c.channelId === channelId);
      if (channel) {
        updateChannel.mutate({
          body: {
            channelId: channel.channelId,
            channelName: channel.channelName,
          },
        });
      }
    },
    [channelData, updateChannel],
  );

  const [botOpen, setBotOpen] = useState(false);
  const [channelOpen, setChannelOpen] = useState(false);
  const [channelOperation, setChannelOperation] = useState<"add" | "delete">(
    "add",
  );
  const [channelID, setChannelID] = useState(0);

  return (
    <div
      className={clsx(
        "flex flex-col gap-6 p-4 w-full h-full overflow-y-auto",
        scrollbarClasses,
      )}
    >
      <div className="bg-surface-container-low rounded-3xl p-6 border border-outline-variant/50">
        <div className="flex items-start gap-4">
          <div className="p-3 rounded-2xl bg-secondary-container">
            <MaterialSymbolsSmartToy className="size-6 text-on-secondary-container" />
          </div>
          <div className="flex-1 min-w-0">
            <h3 className="text-xl font-semibold mb-1">Administrar bots</h3>
            <p className="text-sm text-on-surface-variant">
              Añade varios bots para aumentar la velocidad de carga.
            </p>
          </div>
        </div>
        <div className="mt-6 space-y-4">
          <form
            onSubmit={handleSubmit(onSubmit)}
            className="flex flex-col gap-4"
          >
            <Controller
              name="tokens"
              control={control}
              rules={{ required: true, validate: validateBots }}
              render={({ field, fieldState: { error } }) => (
                <Textarea
                  {...field}
                  disableAutosize
                  classNames={{
                    input: "h-32 min-h-[8rem]",
                    inputWrapper:
                      "bg-surface-container data-[hover=true]:bg-surface-container-high group-data-[focus=true]:bg-surface-container-high border-none transition-colors",
                  }}
                  placeholder="Introduce los tokens (uno por línea)"
                  autoComplete="off"
                  errorMessage={error ? error.message : ""}
                  isInvalid={!!error}
                />
              )}
            />
            <Button
              isLoading={botAddition.isPending}
              type="submit"
              variant="filled"
              className="self-start px-6"
            >
              Añadir bots
            </Button>
          </form>

          <div className="mt-8 pt-6 border-t border-outline-variant/30">
            <div className="flex justify-between items-center">
              <div>
                <p className="text-sm font-medium text-on-surface-variant">
                  Bots activos
                </p>
                <p className="text-3xl font-bold mt-1 text-primary">
                  {userConfig?.bots.length || 0}
                </p>
              </div>
              <BotOperationModal
                open={botOpen}
                handleClose={() => setBotOpen(false)}
                onRemove={handleRemoveBots}
              />
              <div className="flex flex-col sm:flex-row gap-2">
                <Button
                  startContent={<IcRoundContentCopy className="size-4" />}
                  variant="filledTonal"
                  className="text-sm font-medium"
                  onPress={copyTokens}
                  isDisabled={userConfig?.bots.length === 0}
                >
                  Copiar todos
                </Button>
                <Button
                  startContent={
                    <IcRoundRemoveCircleOutline className="size-4" />
                  }
                  variant="filledTonal"
                  className="text-sm font-medium bg-error-container text-on-error-container data-[hover=true]:bg-error-container/80 transition-colors"
                  onPress={() => setBotOpen(true)}
                  isDisabled={userConfig?.bots.length === 0}
                >
                  Eliminar todos
                </Button>
              </div>
            </div>
          </div>
        </div>
      </div>

      <div className="bg-surface-container-low rounded-3xl p-6 border border-outline-variant/50 flex flex-col gap-6">
        <div className="flex flex-col sm:flex-row sm:justify-between sm:items-start gap-3">
          <div className="flex gap-4 items-start">
            <div className="p-3 rounded-2xl bg-secondary-container text-on-secondary-container">
              <MaterialSymbolsTv className="size-6" />
            </div>
            <div className="flex-1 min-w-0">
              <h3 className="text-xl font-semibold mb-1">Canales</h3>
              <p className="text-sm text-on-surface-variant">
                Administra los canales de destino.
              </p>
            </div>
          </div>
          <ChannelOperationModal
            open={channelOpen}
            handleClose={() => setChannelOpen(false)}
            operation={channelOperation}
            channelId={channelID}
          />
          <div className="flex gap-2">
            <Button
              isIconOnly
              variant="text"
              className="text-on-surface-variant"
              onPress={() => {
                setChannelOperation("add");
                setChannelOpen(true);
              }}
            >
              <AddIcon className="size-6" />
            </Button>
            <Button
              isIconOnly
              variant="text"
              className="text-on-surface-variant"
              isLoading={syncChannels.isPending}
              onPress={() => syncChannels.mutate({})}
            >
              <SyncIcon
                className={clsx(
                  "size-6",
                  syncChannels.isPending && "animate-spin",
                )}
              />
            </Button>
          </div>
        </div>

        <div>
          {channelData && channelData.length > 0 ? (
            <RadioGroup
              aria-label="Seleccionar canal predeterminado"
              value={userConfig.channelId?.toString() || ""}
              onValueChange={(value) => handleSetDefaultChannel(Number(value))}
              classNames={{ wrapper: "gap-3 max-h-80 overflow-y-auto pr-2" }}
            >
              {channelData.map((channel) => (
                <div
                  key={channel.channelId}
                  className="flex justify-between items-center p-4 rounded-2xl bg-surface-container hover:bg-surface-container-high transition-colors border border-transparent hover:border-outline-variant/30"
                >
                  <div className="flex-1 flex flex-col">
                    <Radio
                      value={channel.channelId!.toString()}
                      classNames={{ label: "text-base font-semibold" }}
                    >
                      {channel.channelName}
                    </Radio>
                    <p className="text-sm text-on-surface-variant ml-8 mt-0.5 font-mono">
                      ID: {channel.channelId}
                    </p>
                  </div>
                  <Button
                    isIconOnly
                    variant="text"
                    className="text-on-surface-variant data-[hover=true]:text-error transition-colors"
                    onPress={() => {
                      setChannelOperation("delete");
                      setChannelID(channel.channelId!);
                      setChannelOpen(true);
                    }}
                  >
                    <DeleteIcon className="size-5" />
                  </Button>
                </div>
              ))}
            </RadioGroup>
          ) : (
            <div className="flex flex-col items-center justify-center py-10 text-center bg-surface-container/50 rounded-2xl border-2 border-dashed border-outline-variant/30">
              <p className="text-sm text-on-surface-variant max-w-xs">
                No hay canales. Pulsa Sincronizar para obtener tus canales de Telegram.
              </p>
            </div>
          )}
        </div>
      </div>

      <div className="bg-surface-container-low rounded-3xl p-6 border border-outline-variant/50">
        <div className="flex items-start gap-4 mb-6">
          <div className="p-3 rounded-2xl bg-secondary-container text-on-secondary-container">
            <IcRoundSecurity className="size-6" />
          </div>
          <div className="flex-1 min-w-0">
            <h3 className="text-xl font-semibold mb-1">Sesiones activas</h3>
            <p className="text-sm text-on-surface-variant">
              Dispositivos con una sesión iniciada en tu cuenta.
            </p>
          </div>
        </div>
        <div className="grid grid-cols-[repeat(auto-fit,minmax(280px,1fr))] gap-4">
          {sessions?.map((session) => (
            <Session key={session.hash} {...session} />
          ))}
        </div>
      </div>
    </div>
  );
});
