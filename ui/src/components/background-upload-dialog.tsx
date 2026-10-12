import {
  Accordion,
  Button,
  Input,
  Label,
  NumberField,
  Switch,
  TextArea,
  TextField,
} from "@heroui/react";
import { useEffect, useState } from "react";
import { toast } from "sonner";
import PlusIcon from "~icons/gravity-ui/plus";
import TrashIcon from "~icons/gravity-ui/trash-bin";
import type { components } from "@/api/schema";
import { fetchClient } from "@/api/client";
import { newClientId } from "@/features/shared/client-id";
import { useAppLocale } from "@/lib/locale";
import { AppDialog } from "./dialogs/app-dialog";

type ImportSource = components["schemas"]["UploadImportSource"];
type ImportRequest = components["schemas"]["UploadImportRequest"];

type SourceDraft = {
  id: string;
  type: "local" | "http";
  location: string;
  destinationPath: string;
  exclude: string;
  headers: string;
};

const newSource = (): SourceDraft => ({
  id: newClientId(),
  type: "local",
  location: "",
  destinationPath: "",
  exclude: "",
  headers: "",
});

export function BackgroundUploadDialog({
  open,
  onOpenChange,
  currentPath,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  currentPath: string;
}) {
  const [destination, setDestination] = useState(currentPath);
  const [sources, setSources] = useState<SourceDraft[]>([newSource()]);
  const [exclude, setExclude] = useState("");
  const [headers, setHeaders] = useState("");
  const [minSize, setMinSize] = useState("");
  const [maxSize, setMaxSize] = useState("");
  const [chunkSizeMiB, setChunkSizeMiB] = useState(512);
  const [partConcurrency, setPartConcurrency] = useState(4);
  const [encryption, setEncryption] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const { t } = useAppLocale();

  useEffect(() => {
    if (open) setDestination(currentPath);
  }, [currentPath, open]);

  const reset = () => {
    setSources([newSource()]);
    setDestination(currentPath);
    setExclude("");
    setHeaders("");
    setMinSize("");
    setMaxSize("");
    setChunkSizeMiB(512);
    setPartConcurrency(4);
    setEncryption(false);
  };

  const close = () => {
    if (submitting) return;
    onOpenChange(false);
  };

  const patchSource = (id: string, patch: Partial<SourceDraft>) => {
    setSources((items) => items.map((item) => (item.id === id ? { ...item, ...patch } : item)));
  };

  const queueUpload = async () => {
    try {
      const target = destination.trim();
      if (!target) throw new Error(t("uploads.destRequired"));
      if (!target.startsWith("/") && !isUUID(target)) {
        throw new Error(t("uploads.destInvalid"));
      }
      const bodySources = sources.map<ImportSource>((source, index) => {
        const location = source.location.trim();
        if (!location) throw new Error(t("uploads.sourceEmpty", { index: index + 1 }));
        if (source.type === "local" && !location.startsWith("/")) {
          throw new Error(t("uploads.localAbsolute", { index: index + 1 }));
        }
        if (source.type === "http") {
          const url = new URL(location);
          if (url.protocol !== "http:" && url.protocol !== "https:") {
            throw new Error(t("uploads.httpInvalid", { index: index + 1 }));
          }
        }
        const destinationPath = source.destinationPath.trim();
        if (destinationPath.startsWith("/") || destinationPath.split("/").includes("..")) {
          throw new Error(t("uploads.relDest", { index: index + 1 }));
        }
        const item: ImportSource = {
          type: source.type,
          destinationPath: destinationPath || undefined,
          exclude: lines(source.exclude),
          headers: parseHeaders(source.headers, t),
        };
        if (source.type === "local") item.path = location;
        else item.url = location;
        return item;
      });
      const body: ImportRequest = {
        destination: target,
        sources: bodySources,
        headers: parseHeaders(headers, t),
        exclude: lines(exclude),
        minSize: minSize.trim() || undefined,
        maxSize: maxSize.trim() || undefined,
        partConcurrency,
        chunkSize: chunkSizeMiB * 1024 * 1024,
        encryption,
      };
      setSubmitting(true);
      const { error } = await fetchClient.POST("/v1/uploads/imports", { body });
      if (error) throw new Error(t("uploads.rejected"));
      toast.success(t("uploads.queued"));
      reset();
      onOpenChange(false);
    } catch (error) {
      toast.error(error instanceof Error ? error.message : t("uploads.queueFailed"));
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <AppDialog
      open={open}
      onOpenChange={(next) => (next ? onOpenChange(true) : close())}
      isDismissable={!submitting}
      title={t("uploads.bgTitle")}
      description={t("uploads.bgDesc", { path: currentPath })}
      className="min-w-0 sm:w-[min(94vw,46rem)] sm:max-w-none bg-surface"
      bodyClassName="p-0"
      footer={
        <>
          <Button variant="secondary" isDisabled={submitting} onPress={close}>
            {t("dialog.cancel")}
          </Button>
          <Button variant="primary" isPending={submitting} onPress={() => void queueUpload()}>
            {t("uploads.queueUpload")}
          </Button>
        </>
      }
    >
      <div className="grid gap-4 p-4 sm:p-5">
        <TextField value={destination} onChange={setDestination} isRequired>
          <Label>{t("uploads.destination")}</Label>
          <Input placeholder="/Movies/Incoming or a folder UUID" />
          <div className="mt-1 text-xs text-muted">{t("uploads.destHint")}</div>
        </TextField>

        <div className="grid gap-3">
          {sources.map((source, index) => (
            <section
              key={source.id}
              className="rounded-xl border border-border bg-default/15 p-3 sm:p-4"
            >
              <div className="mb-3 flex items-center justify-between gap-3">
                <div>
                  <div className="text-xs font-semibold uppercase tracking-[0.12em] text-muted">
                    {t("uploads.sourceN", { index: index + 1 })}
                  </div>
                  <div className="mt-0.5 text-xs text-muted">
                    {source.type === "local" ? t("uploads.sourceLocal") : t("uploads.sourceHttp")}
                  </div>
                </div>
                <Button
                  isIconOnly
                  size="sm"
                  variant="ghost"
                  aria-label={t("uploads.removeSource", { index: index + 1 })}
                  isDisabled={sources.length === 1}
                  onPress={() =>
                    setSources((items) => items.filter((item) => item.id !== source.id))
                  }
                >
                  <TrashIcon className="size-3.5" />
                </Button>
              </div>

              <div className="mb-3 grid grid-cols-2 rounded-lg bg-default/35 p-1">
                {(["local", "http"] as const).map((type) => (
                  <Button
                    key={type}
                    size="sm"
                    variant={source.type === type ? "secondary" : "ghost"}
                    onPress={() => patchSource(source.id, { type, location: "" })}
                  >
                    {type === "local" ? t("uploads.localPath") : t("uploads.httpUrl")}
                  </Button>
                ))}
              </div>

              <div className="grid gap-3 sm:grid-cols-[minmax(0,1.45fr)_minmax(0,1fr)]">
                <TextField
                  value={source.location}
                  onChange={(value) => patchSource(source.id, { location: value })}
                >
                  <Label>
                    {source.type === "local" ? t("uploads.serverPath") : t("uploads.urlLabel")}
                  </Label>
                  <Input
                    placeholder={
                      source.type === "local"
                        ? "/srv/media/photos"
                        : "https://example.com/archive.zip"
                    }
                  />
                </TextField>
                <TextField
                  value={source.destinationPath}
                  onChange={(value) => patchSource(source.id, { destinationPath: value })}
                >
                  <Label>{t("uploads.destPath")}</Label>
                  <Input placeholder={t("uploads.destPathPlaceholder")} />
                </TextField>
              </div>

              <Accordion hideSeparator className="mt-2 w-full">
                <Accordion.Item id={`source-options-${source.id}`}>
                  <Accordion.Heading>
                    <Accordion.Trigger className="rounded-lg text-xs font-medium text-muted hover:text-foreground">
                      {t("uploads.sourceOptions")}
                      <Accordion.Indicator />
                    </Accordion.Trigger>
                  </Accordion.Heading>
                  <Accordion.Panel>
                    <Accordion.Body>
                      <div className="grid gap-3 border-border border-t pt-3 sm:grid-cols-2">
                        <TextField>
                          <Label>{t("uploads.excludePatterns")}</Label>
                          <TextArea
                            value={source.exclude}
                            onChange={(event) =>
                              patchSource(source.id, { exclude: event.currentTarget.value })
                            }
                            placeholder={"*.tmp\n**/.git/**"}
                            rows={3}
                          />
                        </TextField>
                        <TextField isDisabled={source.type !== "http"}>
                          <Label>{t("uploads.httpHeaders")}</Label>
                          <TextArea
                            value={source.headers}
                            onChange={(event) =>
                              patchSource(source.id, { headers: event.currentTarget.value })
                            }
                            placeholder={"Authorization: Bearer …"}
                            rows={3}
                          />
                        </TextField>
                      </div>
                    </Accordion.Body>
                  </Accordion.Panel>
                </Accordion.Item>
              </Accordion>
            </section>
          ))}
        </div>

        <Button variant="secondary" onPress={() => setSources((items) => [...items, newSource()])}>
          <PlusIcon className="size-3.5" /> {t("uploads.addSource")}
        </Button>

        <Accordion
          hideSeparator
          defaultExpandedKeys={["advanced-settings"]}
          className="w-full rounded-xl border border-border"
        >
          <Accordion.Item id="advanced-settings">
            <Accordion.Heading>
              <Accordion.Trigger className="rounded-xl py-3 text-sm font-semibold">
                {t("uploads.advanced")}
                <Accordion.Indicator />
              </Accordion.Trigger>
            </Accordion.Heading>
            <Accordion.Panel>
              <Accordion.Body>
                <div className="grid gap-4 border-border border-t py-4">
                  <div className="grid gap-3 sm:grid-cols-2">
                    <TextField value={minSize} onChange={setMinSize}>
                      <Label>{t("uploads.minSize")}</Label>
                      <Input placeholder={t("uploads.exampleSize", { size: "10 MiB" })} />
                    </TextField>
                    <TextField value={maxSize} onChange={setMaxSize}>
                      <Label>{t("uploads.maxSize")}</Label>
                      <Input placeholder={t("uploads.exampleSize", { size: "20 GiB" })} />
                    </TextField>
                    <NumberField
                      aria-label={t("uploads.chunkAria")}
                      value={chunkSizeMiB}
                      minValue={64}
                      maxValue={2000}
                      onChange={(value) =>
                        setChunkSizeMiB(Math.max(64, Math.min(2000, value ?? 512)))
                      }
                    >
                      <Label>{t("uploads.chunkSize")}</Label>
                      <NumberField.Group>
                        <NumberField.DecrementButton />
                        <NumberField.Input />
                        <span className="pr-2 text-xs text-muted">MiB</span>
                        <NumberField.IncrementButton />
                      </NumberField.Group>
                    </NumberField>
                    <NumberField
                      aria-label={t("uploads.partsAria")}
                      value={partConcurrency}
                      minValue={1}
                      maxValue={16}
                      onChange={(value) =>
                        setPartConcurrency(Math.max(1, Math.min(16, value ?? 4)))
                      }
                    >
                      <Label>{t("uploads.parts")}</Label>
                      <NumberField.Group>
                        <NumberField.DecrementButton />
                        <NumberField.Input />
                        <NumberField.IncrementButton />
                      </NumberField.Group>
                    </NumberField>
                  </div>
                  <div className="grid gap-3 sm:grid-cols-2">
                    <TextField>
                      <Label>{t("uploads.batchExclusions")}</Label>
                      <TextArea
                        value={exclude}
                        onChange={(event) => setExclude(event.currentTarget.value)}
                        placeholder={"*.tmp\n**/node_modules/**"}
                        rows={3}
                      />
                    </TextField>
                    <TextField>
                      <Label>{t("uploads.defaultHeaders")}</Label>
                      <TextArea
                        value={headers}
                        onChange={(event) => setHeaders(event.currentTarget.value)}
                        placeholder={"Authorization: Bearer …"}
                        rows={3}
                      />
                    </TextField>
                  </div>
                  <Switch isSelected={encryption} onChange={setEncryption}>
                    <Switch.Content>
                      <Switch.Control>
                        <Switch.Thumb />
                      </Switch.Control>
                      <Label>{t("uploads.encrypt")}</Label>
                    </Switch.Content>
                  </Switch>
                </div>
              </Accordion.Body>
            </Accordion.Panel>
          </Accordion.Item>
        </Accordion>
      </div>
    </AppDialog>
  );
}

function lines(value: string) {
  const result = value
    .split(/\r?\n/)
    .map((line) => line.trim())
    .filter(Boolean);
  return result.length ? result : undefined;
}

function parseHeaders(
  value: string,
  t: (key: string, vars?: Record<string, string | number>) => string,
) {
  const result: Record<string, string> = {};
  for (const line of lines(value) ?? []) {
    const separator = line.indexOf(":");
    if (separator < 1) throw new Error(t("uploads.invalidHeader", { header: line }));
    const name = line.slice(0, separator).trim();
    const headerValue = line.slice(separator + 1).trim();
    if (!name || !headerValue) throw new Error(t("uploads.invalidHeader", { header: line }));
    result[name] = headerValue;
  }
  return Object.keys(result).length ? result : undefined;
}

function isUUID(value: string) {
  return /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i.test(value);
}
