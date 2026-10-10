import { FbActions } from "@tw-material/file-browser";
import {
  Button,
  Input,
  Modal,
  ModalBody,
  ModalContent,
  ModalFooter,
  ModalHeader,
} from "@tw-material/react";
import { memo, useCallback } from "react";
import { useShallow } from "zustand/react/shallow";
import { ShareFileDialog } from "./share-dialog";

import { CustomActions } from "@/hooks/use-file-action";
import { $api } from "@/utils/api";
import { useModalStore } from "@/utils/stores";
import { useQueryClient } from "@tanstack/react-query";
import { useSearch } from "@tanstack/react-router";
import toast from "react-hot-toast";

type FileModalProps = {
  queryKey: any;
};

interface RenameDialogProps {
  queryKey: any;
  handleClose: () => void;
}

const RenameDialog = memo(({ queryKey, handleClose }: RenameDialogProps) => {
  const queryClient = useQueryClient();
  const updateFiles = $api.useMutation("patch", "/files/{id}", {
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey });
    },
  });
  const { currentFile, actions } = useModalStore(
    useShallow((state) => ({
      currentFile: state.currentFile,
      actions: state.actions,
    })),
  );

  const onRename = useCallback(
    (e: React.FormEvent<HTMLDivElement>) => {
      e.preventDefault();
      updateFiles
        .mutateAsync({
          params: {
            path: {
              id: currentFile.id,
            },
          },
          body: {
            name: currentFile?.name,
          },
        })
        .then(handleClose)
        .catch(() => toast.error("No se pudo cambiar el nombre. Inténtalo de nuevo."));
    },
    [currentFile.name, currentFile.id],
  );

  return (
    <>
      <ModalHeader className="flex flex-col gap-1">Cambiar nombre</ModalHeader>
      <ModalBody as="form" id="rename-form" onSubmit={onRename}>
        <Input
          size="lg"
          variant="bordered"
          classNames={{
            inputWrapper: "border-primary border-large",
          }}
          autoFocus
          aria-label="Nombre del archivo"
          value={currentFile.name}
          onValueChange={(value) => actions.setCurrentFile({ ...currentFile, name: value })}
        />
      </ModalBody>
      <ModalFooter>
        <Button className="font-normal" variant="text" onPress={handleClose}>
          Cancelar
        </Button>
        <Button
          type="submit"
          className="font-normal"
          variant="filledTonal"
          form="rename-form"
          isDisabled={updateFiles.isPending || !currentFile.name}
          isLoading={updateFiles.isPending}
        >
          Guardar
        </Button>
      </ModalFooter>
    </>
  );
});

interface FolderCreateDialogProps {
  queryKey: any;
  handleClose: () => void;
}

const FolderCreateDialog = memo(({ queryKey, handleClose }: FolderCreateDialogProps) => {
  const queryClient = useQueryClient();

  const { path } = useSearch({ from: "/_authed/$view" });

  const createFolder = $api.useMutation("post", "/files", {
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey });
    },
  });
  const { currentFile, actions } = useModalStore(
    useShallow((state) => ({
      currentFile: state.currentFile,
      actions: state.actions,
    })),
  );

  const onCreate = useCallback(
    (e: React.FormEvent<HTMLDivElement>) => {
      e.preventDefault();
      createFolder
        .mutateAsync({
          body: {
            name: currentFile.name,
            type: "folder",
            path: path ? path : "/",
          },
        })
        .then(() => handleClose())
        .catch(() => toast.error("No se pudo crear la carpeta. Inténtalo de nuevo."));
    },
    [currentFile.name, path, createFolder, handleClose],
  );

  return (
    <>
      <ModalHeader className="flex flex-col gap-1">Nueva carpeta</ModalHeader>
      <ModalBody as="form" id="create-folder-form" onSubmit={onCreate}>
        <Input
          size="lg"
          variant="bordered"
          classNames={{
            inputWrapper: "border-primary border-large",
          }}
          placeholder="Nombre de la carpeta"
          aria-label="Nombre de la carpeta"
          autoFocus
          value={currentFile?.name}
          onValueChange={(value) => actions.setCurrentFile({ ...currentFile, name: value })}
        />
      </ModalBody>
      <ModalFooter>
        <Button className="font-normal" variant="text" onPress={handleClose}>
          Cancelar
        </Button>
        <Button
          type="submit"
          form="create-folder-form"
          className="font-normal"
          variant="filledTonal"
          isDisabled={createFolder.isPending || !currentFile.name}
          isLoading={createFolder.isPending}
        >
          {createFolder.isPending ? "Creando…" : "Crear"}
        </Button>
      </ModalFooter>
    </>
  );
});

interface DeleteDialogProps {
  queryKey: any;
  handleClose: () => void;
}

const DeleteDialog = memo(({ handleClose }: DeleteDialogProps) => {
  const queryClient = useQueryClient();

  const deleteFiles = $api.useMutation("post", "/files/delete", {
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["Files_list"] });
      void queryClient.invalidateQueries({ queryKey: ["Lifecycle"] });
    },
  });

  const selectedFiles = useModalStore((state) => state.selectedFiles) as string[];

  const onDelete = useCallback(() => {
    deleteFiles.mutateAsync({ body: { ids: selectedFiles } }).then(handleClose).catch(() => toast.error("No se pudieron eliminar los archivos. Inténtalo de nuevo."));
  }, [selectedFiles, deleteFiles, handleClose]);

  return (
    <>
      <ModalHeader className="flex flex-col gap-1">Mover a la papelera</ModalHeader>
      <ModalBody>
        <h1 className="text-large font-medium mt-2">
          {`¿Mover ${selectedFiles.length} elemento${selectedFiles.length > 1 ? "s" : ""} a la papelera?`}
        </h1>
        <p className="text-sm text-on-surface-variant">Puedes restaurarlos durante 30 días. Después se eliminan durante la limpieza programada del servidor.</p>
      </ModalBody>
      <ModalFooter>
        <Button className="font-normal" variant="text" onPress={handleClose}>
          Cancelar
        </Button>
        <Button
          variant="filledTonal"
          classNames={{
            base: "font-normal",
          }}
          onPress={onDelete}
          isDisabled={deleteFiles.isPending}
          isLoading={deleteFiles.isPending}
        >
          Mover a la papelera
        </Button>
      </ModalFooter>
    </>
  );
});

export const FileOperationModal = memo(({ queryKey }: FileModalProps) => {
  const { open, operation, actions } = useModalStore(
    useShallow((state) => ({
      open: state.open,
      operation: state.operation,
      actions: state.actions,
    })),
  );

  const handleClose = useCallback(
    () =>
      actions.set({
        open: false,
      }),
    [],
  );

  const renderOperation = () => {
    switch (operation) {
      case FbActions.RenameFile.id:
        return <RenameDialog queryKey={queryKey} handleClose={handleClose} />;
      case FbActions.CreateFolder.id:
        return <FolderCreateDialog queryKey={queryKey} handleClose={handleClose} />;
      case FbActions.DeleteFiles.id:
        return <DeleteDialog queryKey={queryKey} handleClose={handleClose} />;
      case CustomActions.ShareFiles.id:
        return <ShareFileDialog handleClose={handleClose} />;
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
});
