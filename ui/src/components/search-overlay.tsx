import { Button, Chip, Input, Kbd, Spinner } from "@heroui/react";
import { useNavigate } from "@tanstack/react-router";
import { useEffect, useRef, useState } from "react";
import FileIcon from "~icons/gravity-ui/file";
import FolderIcon from "~icons/gravity-ui/folder";
import SearchIcon from "~icons/gravity-ui/magnifier";
import { $api } from "@/api/client";
import type { FileEntry } from "@/api/types";
import { useCommandPalette } from "./command-palette-context";

export function SearchOverlay() {
  const { isOpen, close } = useCommandPalette();
  const [query, setQuery] = useState("");
  const inputRef = useRef<HTMLInputElement>(null);
  const navigate = useNavigate();
  const [selectedIndex, setSelectedIndex] = useState(0);

  useEffect(() => {
    if (!isOpen) return;
    setQuery("");
    setSelectedIndex(0);
    window.setTimeout(() => inputRef.current?.focus(), 50);
  }, [isOpen]);

  const { data, isFetching } = $api.useQuery(
    "get",
    "/v1/files",
    {
      params: {
        query: {
          search: query,
          searchType: "text",
          status: "active",
          limit: 50,
          sort: "name",
          order: "asc",
        },
      },
    },
    { enabled: isOpen && query.trim().length >= 2, staleTime: 10_000 },
  );

  const items = (data?.items ?? []) as FileEntry[];

  const openItem = async (item: FileEntry) => {
    close();
    if (item.kind === "folder") {
      await navigate({
        to: "/files",
        search: {
          path: "/",
          parentId: item.id,
          query: "",
          view: "list",
        },
      });
      return;
    }
    await navigate({
      to: "/files",
      search: {
        path: "/",
        parentId: item.parentId,
        query: item.name,
        view: "list",
      },
    });
  };

  const handleKeyDown = (event: React.KeyboardEvent) => {
    if (event.key === "ArrowDown") {
      event.preventDefault();
      setSelectedIndex((index) => Math.min(index + 1, Math.max(items.length - 1, 0)));
    } else if (event.key === "ArrowUp") {
      event.preventDefault();
      setSelectedIndex((index) => Math.max(index - 1, 0));
    } else if (event.key === "Enter" && items[selectedIndex]) {
      event.preventDefault();
      void openItem(items[selectedIndex]);
    }
  };

  if (!isOpen) return null;

  return (
    <div className="fixed inset-0 z-50 flex items-start justify-center pt-[15vh]">
      <Button
        type="button"
        variant="ghost"
        aria-label="Close search"
        className="absolute inset-0 h-full w-full rounded-none bg-black/60 backdrop-blur-sm"
        onPress={close}
      />
      <div
        role="dialog"
        aria-modal="true"
        aria-label="Search files"
        className="relative z-10 flex w-full max-w-lg flex-col overflow-hidden rounded-2xl border border-border bg-surface shadow-2xl"
      >
        <div className="flex items-center gap-3 border-b border-border px-4 py-3">
          <SearchIcon className="size-5 shrink-0 text-muted" />
          <Input
            ref={inputRef}
            aria-label="Search files and folders"
            value={query}
            onChange={(event) => {
              setQuery(event.currentTarget.value);
              setSelectedIndex(0);
            }}
            onKeyDown={handleKeyDown}
            placeholder="Search files and folders..."
            className="flex-1 bg-transparent text-sm outline-none placeholder:text-muted"
          />
          {query && (
            <Button
              size="sm"
              variant="tertiary"
              className="h-auto min-w-0 p-0 text-xs text-muted hover:text-foreground"
              onPress={() => setQuery("")}
            >
              Clear
            </Button>
          )}
          <Kbd key="esc" className="text-[10px]">
            esc
          </Kbd>
        </div>
        <div className="max-h-[50vh] overflow-y-auto py-2">
          {query.trim().length < 2 ? (
            <div className="px-4 py-8 text-center text-xs text-muted">
              Type at least 2 characters to search
            </div>
          ) : isFetching ? (
            <div className="flex items-center justify-center py-8">
              <Spinner size="sm" />
            </div>
          ) : items.length === 0 ? (
            <div className="px-4 py-8 text-center text-xs text-muted">No results for “{query}”</div>
          ) : (
            items.map((item, index) => {
              const Icon = item.kind === "folder" ? FolderIcon : FileIcon;
              return (
                <Button
                  key={item.id}
                  variant="ghost"
                  className={`h-auto w-full justify-start gap-3 px-4 py-2.5 text-left ${index === selectedIndex ? "bg-accent/10" : ""}`}
                  onPress={() => void openItem(item)}
                  onMouseEnter={() => setSelectedIndex(index)}
                >
                  <div className="flex size-9 shrink-0 items-center justify-center rounded-lg bg-accent/10 text-accent">
                    <Icon className="size-4" />
                  </div>
                  <div className="min-w-0 flex-1">
                    <p className="truncate text-sm font-medium">{item.name}</p>
                    <p className="truncate text-xs text-muted">
                      {item.kind === "folder" ? "Folder" : item.mimeType || "File"}
                    </p>
                  </div>
                  <Chip size="sm" variant="tertiary" className="shrink-0 capitalize">
                    {item.kind}
                  </Chip>
                </Button>
              );
            })
          )}
        </div>
      </div>
    </div>
  );
}
