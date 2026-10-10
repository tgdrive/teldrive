import { useFileUploadStore } from "@/utils/stores";
import clsx from "clsx";
import { AnimatePresence, motion } from "framer-motion";
import { memo, useCallback, useState } from "react";
import IconParkOutlineUpload from "~icons/icon-park-outline/upload";

interface UploadDropzoneProps {
  children: React.ReactNode;
  isDisabled: boolean;
}

export const UploadDropzone = memo(
  ({ children, isDisabled }: UploadDropzoneProps) => {
    const [isDragging, setIsDragging] = useState(false);
    const handleDragDrop = useFileUploadStore(
      (state) => state.actions.handleDragDrop,
    );
    const addFiles = useFileUploadStore((state) => state.actions.addFiles);
    const setUploadOpen = useFileUploadStore(
      (state) => state.actions.setUploadOpen,
    );

    const onDragOver = useCallback((e: React.DragEvent) => {
      e.preventDefault();
      e.stopPropagation();
      if (!isDisabled && Array.from(e.dataTransfer.types).includes("Files")) {
        setIsDragging(true);
      }
    }, [isDisabled]);

    const onDragLeave = useCallback((e: React.DragEvent) => {
      e.preventDefault();
      e.stopPropagation();
      // Only set to false if we're leaving the dropzone itself, not entering a child
      if (e.currentTarget.contains(e.relatedTarget as Node)) return;
      setIsDragging(false);
    }, []);

    const onDrop = useCallback(
      async (e: React.DragEvent) => {
        e.preventDefault();
        e.stopPropagation();
        setIsDragging(false);

        if (isDisabled) return;

        const nativeEvent = e.nativeEvent as DragEvent;
        const nativeItems = nativeEvent?.dataTransfer?.items;

        if (nativeItems) {
          await handleDragDrop(nativeItems);
        } else {
          // Fallback for browsers that don't support DataTransferItem or entries
          const rootFiles: File[] = [];
          // items is a DataTransferItemList, we need to iterate it
          if (e.dataTransfer.items) {
            for (let i = 0; i < e.dataTransfer.items.length; i++) {
              const item = e.dataTransfer.items[i];
              if (item.kind === "file") {
                const file = item.getAsFile();
                if (file) rootFiles.push(file);
              }
            }
          } else if (e.dataTransfer.files) {
            for (let i = 0; i < e.dataTransfer.files.length; i++) {
              rootFiles.push(e.dataTransfer.files[i]);
            }
          }

          if (rootFiles.length > 0) {
            addFiles(rootFiles);
            setUploadOpen(true);
          }
        }
      },
      [handleDragDrop, addFiles, setUploadOpen, isDisabled],
    );

    return (
      <div
        className="relative flex-1 min-h-0 w-full outline-none"
        onDragOver={onDragOver}
        onDragLeave={onDragLeave}
        onDrop={onDrop}
      >
        {children}
        <AnimatePresence>
          {isDragging && !isDisabled && (
            <motion.div
              initial={{ opacity: 0 }}
              animate={{ opacity: 1 }}
              exit={{ opacity: 0 }}
              className={clsx(
                "absolute inset-0 z-[100] flex items-center justify-center p-6",
                "bg-primary/10 backdrop-blur-[2px] pointer-events-none",
              )}
            >
              <motion.div
                initial={{ scale: 0.9, opacity: 0, y: 10 }}
                animate={{ scale: 1, opacity: 1, y: 0 }}
                exit={{ scale: 0.9, opacity: 0, y: 10 }}
                className={clsx(
                  "bg-surface-container-high border-2 border-dashed border-primary/50",
                  "rounded-[32px] p-12 flex flex-col items-center gap-4 shadow-2xl",
                  "max-w-sm w-full text-center",
                )}
              >
                <div className="size-16 rounded-2xl bg-primary-container flex items-center justify-center text-on-primary-container mb-2">
                  <IconParkOutlineUpload className="size-8" />
                </div>
                <div className="space-y-1">
                  <p className="text-headline-small font-semibold text-on-surface">
                    Suelta los archivos para subirlos
                  </p>
                  <p className="text-body-medium text-on-surface-variant">
                    Los archivos se añadirán a la cola
                  </p>
                </div>
              </motion.div>
            </motion.div>
          )}
        </AnimatePresence>
      </div>
    );
  },
);
