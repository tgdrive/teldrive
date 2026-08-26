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
      if (!target) throw new Error("Destination is required");
      if (!target.startsWith("/") && !isUUID(target)) {
        throw new Error("Destination must be an absolute drive path or folder UUID");
      }
      const bodySources = sources.map<ImportSource>((source, index) => {
        const location = source.location.trim();
        if (!location) throw new Error(`Source ${index + 1} is empty`);
        if (source.type === "local" && !location.startsWith("/")) {
          throw new Error(`Local source ${index + 1} must use an absolute path`);
        }
        if (source.type === "http") {
          const url = new URL(location);
          if (url.protocol !== "http:" && url.protocol !== "https:") {
            throw new Error(`HTTP source ${index + 1} must use http or https`);
          }
        }
        const destinationPath = source.destinationPath.trim();
        if (destinationPath.startsWith("/") || destinationPath.split("/").includes("..")) {
          throw new Error(`Destination ${index + 1} must be a relative path`);
        }
        const item: ImportSource = {
          type: source.type,
          destinationPath: destinationPath || undefined,
          exclude: lines(source.exclude),
          headers: parseHeaders(source.headers),
        };
        if (source.type === "local") item.path = location;
        else item.url = location;
        return item;
      });
      const body: ImportRequest = {
        destination: target,
        sources: bodySources,
        headers: parseHeaders(headers),
        exclude: lines(exclude),
        minSize: minSize.trim() || undefined,
        maxSize: maxSize.trim() || undefined,
        partConcurrency,
        chunkSize: chunkSizeMiB * 1024 * 1024,
        encryption,
      };
      setSubmitting(true);
      const { error } = await fetchClient.POST("/v1/uploads/imports", { body });
      if (error) throw new Error("The server rejected the background upload");
      toast.success("Background upload queued");
      reset();
      onOpenChange(false);
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Failed to queue background upload");
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <AppDialog
      open={open}
      onOpenChange={(next) => (next ? onOpenChange(true) : close())}
      isDismissable={!submitting}
      title="Background upload"
      description={`Import server paths and remote URLs into ${currentPath}.`}
      className="min-w-0 sm:w-[min(94vw,46rem)] sm:max-w-none bg-surface"
      bodyClassName="p-0"
      footer={
        <>
          <Button variant="secondary" isDisabled={submitting} onPress={close}>
            Cancel
          </Button>
          <Button variant="primary" isPending={submitting} onPress={() => void queueUpload()}>
            Queue upload
          </Button>
        </>
      }
    >
      <div className="grid gap-4 p-4 sm:p-5">
        <TextField value={destination} onChange={setDestination} isRequired>
          <Label>Destination</Label>
          <Input placeholder="/Movies/Incoming or a folder UUID" />
          <div className="mt-1 text-xs text-muted">
            Enter an absolute drive path from root or an existing folder UUID.
          </div>
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
                    Source {index + 1}
                  </div>
                  <div className="mt-0.5 text-xs text-muted">
                    {source.type === "local" ? "Read from this server" : "Fetch over HTTP"}
                  </div>
                </div>
                <Button
                  isIconOnly
                  size="sm"
                  variant="ghost"
                  aria-label={`Remove source ${index + 1}`}
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
                    {type === "local" ? "Local path" : "HTTP URL"}
                  </Button>
                ))}
              </div>

              <div className="grid gap-3 sm:grid-cols-[minmax(0,1.45fr)_minmax(0,1fr)]">
                <TextField
                  value={source.location}
                  onChange={(value) => patchSource(source.id, { location: value })}
                >
                  <Label>{source.type === "local" ? "Absolute server path" : "URL"}</Label>
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
                  <Label>Destination path</Label>
                  <Input placeholder="Optional relative path" />
                </TextField>
              </div>

              <Accordion hideSeparator className="mt-2 w-full">
                <Accordion.Item id={`source-options-${source.id}`}>
                  <Accordion.Heading>
                    <Accordion.Trigger className="rounded-lg text-xs font-medium text-muted hover:text-foreground">
                      Source options
                      <Accordion.Indicator />
                    </Accordion.Trigger>
                  </Accordion.Heading>
                  <Accordion.Panel>
                    <Accordion.Body>
                      <div className="grid gap-3 border-border border-t pt-3 sm:grid-cols-2">
                        <TextField>
                          <Label>Exclude patterns</Label>
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
                          <Label>HTTP headers</Label>
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
          <PlusIcon className="size-3.5" /> Add source
        </Button>

        <Accordion
          hideSeparator
          defaultExpandedKeys={["advanced-settings"]}
          className="w-full rounded-xl border border-border"
        >
          <Accordion.Item id="advanced-settings">
            <Accordion.Heading>
              <Accordion.Trigger className="rounded-xl py-3 text-sm font-semibold">
                Advanced settings
                <Accordion.Indicator />
              </Accordion.Trigger>
            </Accordion.Heading>
            <Accordion.Panel>
              <Accordion.Body>
                <div className="grid gap-4 border-border border-t py-4">
                  <div className="grid gap-3 sm:grid-cols-2">
                    <TextField value={minSize} onChange={setMinSize}>
                      <Label>Minimum size</Label>
                      <Input placeholder="For example 10 MiB" />
                    </TextField>
                    <TextField value={maxSize} onChange={setMaxSize}>
                      <Label>Maximum size</Label>
                      <Input placeholder="For example 20 GiB" />
                    </TextField>
                    <NumberField
                      aria-label="Chunk size in MiB"
                      value={chunkSizeMiB}
                      minValue={64}
                      maxValue={2000}
                      onChange={(value) =>
                        setChunkSizeMiB(Math.max(64, Math.min(2000, value ?? 512)))
                      }
                    >
                      <Label>Chunk size (MiB)</Label>
                      <NumberField.Group>
                        <NumberField.DecrementButton />
                        <NumberField.Input />
                        <span className="pr-2 text-xs text-muted">MiB</span>
                        <NumberField.IncrementButton />
                      </NumberField.Group>
                    </NumberField>
                    <NumberField
                      aria-label="Concurrent upload parts"
                      value={partConcurrency}
                      minValue={1}
                      maxValue={16}
                      onChange={(value) =>
                        setPartConcurrency(Math.max(1, Math.min(16, value ?? 4)))
                      }
                    >
                      <Label>Concurrent parts</Label>
                      <NumberField.Group>
                        <NumberField.DecrementButton />
                        <NumberField.Input />
                        <NumberField.IncrementButton />
                      </NumberField.Group>
                    </NumberField>
                  </div>
                  <div className="grid gap-3 sm:grid-cols-2">
                    <TextField>
                      <Label>Batch exclusions</Label>
                      <TextArea
                        value={exclude}
                        onChange={(event) => setExclude(event.currentTarget.value)}
                        placeholder={"*.tmp\n**/node_modules/**"}
                        rows={3}
                      />
                    </TextField>
                    <TextField>
                      <Label>Default HTTP headers</Label>
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
                      <Label>Encrypt uploaded files</Label>
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

function parseHeaders(value: string) {
  const result: Record<string, string> = {};
  for (const line of lines(value) ?? []) {
    const separator = line.indexOf(":");
    if (separator < 1) throw new Error(`Invalid header: ${line}`);
    const name = line.slice(0, separator).trim();
    const headerValue = line.slice(separator + 1).trim();
    if (!name || !headerValue) throw new Error(`Invalid header: ${line}`);
    result[name] = headerValue;
  }
  return Object.keys(result).length ? result : undefined;
}

function isUUID(value: string) {
  return /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i.test(value);
}
