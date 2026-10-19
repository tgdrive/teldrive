import { t } from "@/lib/localization";
import { Button, Checkbox, Input, Label, ListBox, Popover, Select, TextField } from "@heroui/react";
import { useEffect, useState } from "react";
import { FolderPicker } from "./folder-picker";
import { FileSortControls } from "./file-sort-controls";
import { validFileSort } from "./sort-preference";
import { invalidSearchDates, type SearchState, searchCategories, searchDate } from "./search-state";

export function SearchControls({
  search,
  onChange,
  isOpen,
  onOpenChange,
  status,
}: {
  search: SearchState;
  onChange: (search: SearchState) => void;
  isOpen: boolean;
  onOpenChange: (open: boolean) => void;
  status?: string;
}) {
  const [draft, setDraft] = useState(search);

  useEffect(() => setDraft(search), [search, isOpen]);
  const invalidDates = invalidSearchDates(draft);
  const missingFolder = draft.scope === "recursive" && !draft.parentId;
  const setFilter = <K extends keyof SearchState>(key: K, value: SearchState[K]) =>
    setDraft((previous) => ({ ...previous, [key]: value }));
  const clear = () => {
    onChange({ q: search.q, sort: search.sort, order: search.order, view: search.view });
    onOpenChange(false);
  };
  const remove = (key: keyof SearchState) => onChange({ ...search, [key]: undefined });
  const filtered = Boolean(
    search.kind ||
      search.category?.length ||
      search.updatedAfter ||
      search.updatedBefore ||
      search.scope === "recursive",
  );

  return (
    <>
      <div className="flex min-w-0 flex-wrap items-center gap-2 [&>button]:shrink-0">
        <h1 className="sr-only">{t("Resultados de búsqueda")}</h1>
        <Popover isOpen={isOpen} onOpenChange={onOpenChange}>
          <Button variant="secondary" size="sm">
            {t("Filtros")}
          </Button>
          <Popover.Content placement="bottom end" className="w-[min(92vw,26rem)]">
            <Popover.Dialog className="max-h-[min(80dvh,44rem)] overflow-y-auto p-4">
              <form
                className="space-y-4"
                onSubmit={(event) => {
                  event.preventDefault();
                  if (invalidDates || missingFolder) return;
                  onChange({ ...draft, q: search.q });
                  onOpenChange(false);
                }}
              >
                <Popover.Heading className="text-sm font-semibold">
                  {t("Refina la búsqueda")}
                </Popover.Heading>
                <SearchSelect
                  label={t("Buscar en")}
                  value={draft.scope ?? "drive"}
                  onChange={(scope) => setFilter("scope", scope)}
                  options={[
                    { value: "drive", label: t("Todos mis archivos") },
                    { value: "recursive", label: t("Carpeta y subcarpetas") },
                  ]}
                />
                {draft.scope === "recursive" &&
                  (draft.parentId ? (
                    <div className="flex items-center justify-between gap-2 text-xs text-muted">
                      <span className="min-w-0 truncate" title={draft.folderPath}>
                        {t("Folder: {path}", {
                          path: draft.folderPath ?? t("search.folderUnnamed"),
                        })}
                      </span>
                      <Button
                        size="sm"
                        variant="ghost"
                        onPress={() =>
                          setDraft((previous) => ({
                            ...previous,
                            parentId: undefined,
                            folderPath: undefined,
                          }))
                        }
                      >
                        {t("Cambiar carpeta")}
                      </Button>
                    </div>
                  ) : (
                    <div className="rounded-xl border border-border p-3">
                      <p className="mb-2 text-xs text-muted">
                        {t("Elige la carpeta donde buscar.")}
                      </p>
                      <FolderPicker
                        confirmLabel={t("Usar esta carpeta")}
                        requireFolder
                        onConfirm={(parentId, path) => {
                          if (parentId)
                            setDraft((previous) => ({ ...previous, parentId, folderPath: path }));
                        }}
                      />
                    </div>
                  ))}
                <SearchSelect
                  label={t("Tipo")}
                  value={draft.kind ?? "all"}
                  onChange={(kind) => setFilter("kind", kind === "all" ? undefined : kind)}
                  options={[
                    { value: "all", label: t("Archivos y carpetas") },
                    { value: "file", label: t("Archivos") },
                    { value: "folder", label: t("Carpetas") },
                  ]}
                />
                <fieldset className="grid grid-cols-2 gap-2">
                  <legend className="mb-2 text-sm">{t("Categorías")}</legend>
                  {searchCategories.map((category) => (
                    <Checkbox
                      key={category}
                      isSelected={draft.category?.includes(category) ?? false}
                      onChange={(checked) =>
                        setFilter(
                          "category",
                          checked
                            ? [...(draft.category ?? []), category]
                            : draft.category?.filter((value) => value !== category),
                        )
                      }
                    >
                      <Checkbox.Control>
                        <Checkbox.Indicator />
                      </Checkbox.Control>
                      <Label className="capitalize">{category}</Label>
                    </Checkbox>
                  ))}
                </fieldset>
                <div className="grid grid-cols-2 gap-3">
                  <TextField
                    value={draft.updatedAfter?.slice(0, 10) ?? ""}
                    onChange={(value) => setFilter("updatedAfter", searchDate(value))}
                  >
                    <Label className="text-xs">{t("Modificado después de")}</Label>
                    <Input type="date" />
                  </TextField>
                  <TextField
                    value={draft.updatedBefore?.slice(0, 10) ?? ""}
                    onChange={(value) => setFilter("updatedBefore", searchDate(value))}
                  >
                    <Label className="text-xs">{t("Modificado antes de")}</Label>
                    <Input type="date" />
                  </TextField>
                </div>
                {invalidDates && (
                  <p role="alert" className="text-xs text-danger">
                    {t("La fecha inicial debe ser anterior a la final.")}
                  </p>
                )}
                {missingFolder && (
                  <p role="alert" className="text-xs text-danger">
                    {t("Elige una carpeta antes de aplicar este ámbito.")}
                  </p>
                )}
                <div className="flex justify-between">
                  <Button variant="ghost" size="sm" onPress={clear}>
                    {t("Borrar filtros")}
                  </Button>
                  <Button
                    type="submit"
                    variant="primary"
                    size="sm"
                    isDisabled={invalidDates || missingFolder}
                  >
                    {t("Aplicar filtros")}
                  </Button>
                </div>
              </form>
            </Popover.Dialog>
          </Popover.Content>
        </Popover>
        <FileSortControls
          sorting={{ sort: validFileSort(search.sort) ?? "name", order: search.order ?? "asc" }}
          onChange={(sorting) => onChange({ ...search, ...sorting })}
        />
        {status && (
          <span aria-live="polite" className="whitespace-nowrap text-xs text-muted">
            {status}
          </span>
        )}
      </div>
      {(search.q || filtered) && (
        <section
          className="flex min-w-0 max-w-full flex-wrap items-center gap-2"
          aria-label={t("Filtros activos")}
        >
          {search.q && (
            <FilterChip label={t("Name: {q}", { q: search.q })} onRemove={() => remove("q")} />
          )}
          {search.scope === "recursive" && (
            <FilterChip
              label={t("In {folder}", {
                folder: search.folderPath ?? t("search.folderFallback"),
              })}
              onRemove={() =>
                onChange({ ...search, scope: "drive", parentId: undefined, folderPath: undefined })
              }
            />
          )}
          {search.kind && (
            <FilterChip
              label={t(search.kind === "file" ? "search.typeFile" : "search.typeFolder")}
              onRemove={() => remove("kind")}
            />
          )}
          {search.category?.map((category) => (
            <FilterChip
              key={category}
              label={category}
              onRemove={() =>
                onChange({
                  ...search,
                  category: search.category?.filter((value) => value !== category),
                })
              }
            />
          ))}
          {search.updatedAfter && (
            <FilterChip
              label={t("After {date}", { date: search.updatedAfter.slice(0, 10) })}
              onRemove={() => remove("updatedAfter")}
            />
          )}
          {search.updatedBefore && (
            <FilterChip
              label={t("Before {date}", { date: search.updatedBefore.slice(0, 10) })}
              onRemove={() => remove("updatedBefore")}
            />
          )}
          {filtered && (
            <Button size="sm" variant="ghost" onPress={clear}>
              {t("Borrar filtros")}
            </Button>
          )}
        </section>
      )}
    </>
  );
}

function SearchSelect<T extends string>({
  label,
  value,
  options,
  onChange,
}: {
  label: string;
  value: T;
  options: { value: T; label: string }[];
  onChange: (value: T) => void;
}) {
  return (
    <Select
      className="w-full"
      selectedKey={value}
      onSelectionChange={(key) => {
        const option = options.find((item) => item.value === key);
        if (option) onChange(option.value);
      }}
    >
      <Label className="text-sm">{label}</Label>
      <Select.Trigger>
        <Select.Value />
        <Select.Indicator />
      </Select.Trigger>
      <Select.Popover>
        <ListBox>
          {options.map((option) => (
            <ListBox.Item key={option.value} id={option.value} textValue={option.label}>
              {option.label}
              <ListBox.ItemIndicator />
            </ListBox.Item>
          ))}
        </ListBox>
      </Select.Popover>
    </Select>
  );
}

function FilterChip({ label, onRemove }: { label: string; onRemove: () => void }) {
  return (
    <Button
      size="sm"
      variant="secondary"
      className="max-w-full rounded-full"
      onPress={onRemove}
      aria-label={t("Remove {label} filter", { label })}
    >
      <span className="truncate">{label}</span>
      <span aria-hidden="true">×</span>
    </Button>
  );
}
