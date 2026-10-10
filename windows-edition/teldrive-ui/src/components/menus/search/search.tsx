import { type NavigateOptions, useRouter, useRouterState } from "@tanstack/react-router";
import {
  Button,
  Checkbox,
  CheckboxGroup,
  Input,
  FreeSoloPopover as Popover,
  Radio,
  RadioGroup,
} from "@tw-material/react";
import clsx from "clsx";
import { AnimatePresence, motion } from "framer-motion";
import { memo, useCallback, useEffect, useRef, useState } from "react";
import { Controller, useForm, useWatch } from "react-hook-form";
import { useLocalStorage } from "usehooks-ts";
import IconFaSolidFile from "~icons/fa-solid/file";
import IconFaSolidFileArchive from "~icons/fa-solid/file-archive";
import IconFaSolidFileImage from "~icons/fa-solid/file-image";
import IconFaSolidFilePdf from "~icons/fa-solid/file-pdf";
import IconFa6SolidFileVideo from "~icons/fa6-solid/file-video";
import IconIcBaselineHistory from "~icons/ic/baseline-history";
import IconIcOutlineFolderOpen from "~icons/ic/outline-folder-open";
import IconIconamoonMusic1Bold from "~icons/iconamoon/music-1-bold";
import IconMaterialSymbolsClose from "~icons/material-symbols/close";

import { scrollbarClasses } from "@/utils/classes";
import { categoryLabels } from "@/utils/locale";

import type { FileListParams } from "@/types";
import { FilterChip } from "./filter-chip";

const getCurrentDateFormatted = () => {
  const today = new Date();
  const year = today.getFullYear();
  const month = String(today.getMonth() + 1).padStart(2, "0");
  const day = String(today.getDate()).padStart(2, "0");
  return `${year}-${month}-${day}`;
};

const categories = [
  { value: "archive", icon: IconFaSolidFileArchive },
  { value: "audio", icon: IconIconamoonMusic1Bold },
  { value: "image", icon: IconFaSolidFileImage },
  { value: "video", icon: IconFa6SolidFileVideo },
  { value: "document", icon: IconFaSolidFilePdf },
  { value: "folder", icon: IconIcOutlineFolderOpen },
  { value: "other", icon: IconFaSolidFile },
];

const searchTypes = ["text", "regex"];

const locations = ["current", "custom"];

const modifiedDateValues = [
  { value: "7", label: "Últimos 7 días" },
  { value: "30", label: "Últimos 30 días" },
  { value: "90", label: "Últimos 90 días" },
  { value: "-2", label: "Personalizado" },
];

interface SearchMenuProps {
  isOpen: boolean;
  setIsOpen: (open: boolean) => void;
  triggerRef: React.RefObject<HTMLButtonElement>;
}

const defaultFilters = {
  category: [] as string[],
  location: "",
  modifiedDate: "",
  query: "",
  fromDate: "",
  toDate: "",
  path: "",
  deepSearch: false,
  searchType: "text",
};

export const SearchMenu = memo(({ isOpen, setIsOpen, triggerRef }: SearchMenuProps) => {
  const formRef = useRef<HTMLFormElement | null>(null);
  const [recentSearches, setRecentSearches] = useLocalStorage<string[]>("recent-searches", []);

  const { control, handleSubmit, reset, setValue } = useForm({
    defaultValues: defaultFilters,
  });

  const modifiedDate = useWatch({ control, name: "modifiedDate" });
  const fromDate = useWatch({ control, name: "fromDate" });
  const location = useWatch({ control, name: "location" });
  const query = useWatch({ control, name: "query" });

  const pathname = useRouterState({ select: (s) => s.location.pathname });
  const today = getCurrentDateFormatted();
  const router = useRouter();
  const [isSearching, setIsSearching] = useState(false);

  const onSubmit = useCallback(
    (data: typeof defaultFilters) => {
      const filterQuery = {} as FileListParams["params"];

      if (data.query) {
        setRecentSearches((prev) => {
          const filtered = prev.filter((s) => s !== data.query);
          return [data.query, ...filtered].slice(0, 5);
        });
      }

      for (const key in data) {
        const value = data[key];

        if (key === "category" && value.length > 0) {
          const fileCategories = value.filter((category: string) => category !== "folder");
          if (fileCategories.length) filterQuery.category = fileCategories;
          else if (value.includes("folder")) filterQuery.type = "folder";
        } else if (key === "modifiedDate") {
          if (value === "-2") {
            if (data.fromDate) {
              filterQuery.updatedAt = `gte:${data.fromDate}`;
            }
            if (data.toDate) {
              filterQuery.updatedAt = filterQuery.updatedAt
                ? `${filterQuery.updatedAt},lte:${data.toDate}`
                : `lte:${data.toDate}`;
            }
          } else if (Number(value) > 0) {
            const currentDate = new Date();
            currentDate.setUTCDate(currentDate.getUTCDate() - Number(value));
            filterQuery.updatedAt = `gte:${currentDate.toISOString().split("T")[0]}`;
          }
        } else if (key === "location" && value === "current" && pathname.includes("/my-drive")) {
          filterQuery.path = (router.state.location.search as FileListParams["params"]).path || "/";
          filterQuery.deepSearch = data.deepSearch;
        } else if (key === "location" && value === "custom" && data.path) {
          filterQuery.path = data.path;
          filterQuery.deepSearch = data.deepSearch;
        } else if (key === "query" && value) {
          filterQuery[key] = value;
        } else if (key === "searchType" && value) {
          filterQuery[key] = value;
        }
      }
      const nextRoute: NavigateOptions = {
        to: "/$view",
        search: filterQuery,
        params: {
          view: "search",
        },
      };

      if (Object.keys(filterQuery).length === 0) {
        return;
      }
      setIsSearching(true);
      router
        .preloadRoute(nextRoute)
        .then(() => router.navigate(nextRoute).then(() => setIsOpen(false)))
        .finally(() => setIsSearching(false));
    },
    [pathname, setRecentSearches, setIsOpen, router],
  );

  const removeRecentSearch = (search: string) => {
    setRecentSearches((prev) => prev.filter((s) => s !== search));
  };

  return (
    <Popover
      placement="bottom-end"
      isOpen={isOpen}
      onOpenChange={(open) => setIsOpen(open)}
      triggerRef={triggerRef}
      classNames={{
        content:
          "max-w-md max-h-[80vh] justify-normal p-0 rounded-2xl bg-surface-container-high border border-outline-variant/30 shadow-2xl",
      }}
    >
      <div className="flex flex-col h-full overflow-hidden">
        <form
          ref={formRef}
          id="filter-form"
          onSubmit={handleSubmit(onSubmit)}
          className={clsx(
            "flex-1 flex flex-col gap-6 p-6 relative w-full text-on-surface overflow-y-auto",
            scrollbarClasses,
          )}
        >
          {recentSearches.length > 0 && !query && (
            <section className="flex flex-col gap-2">
              <h3 className="text-sm font-medium text-on-surface-variant flex items-center gap-2">
                <IconIcBaselineHistory className="size-4" />
                Búsquedas recientes
              </h3>
              <div className="flex flex-wrap gap-2">
                {recentSearches.map((s) => (
                  <div
                    key={s}
                    className="group flex items-center gap-1 bg-surface-container-high hover:bg-surface-container-highest transition-colors rounded-full pl-3 pr-1 py-1 cursor-pointer"
                    onClick={() => {
                      setValue("query", s);
                      // Trigger submit manually
                      handleSubmit(onSubmit)();
                    }}
                  >
                    <span className="text-sm">{s}</span>
                    <Button
                      isIconOnly
                      variant="text"
                      size="sm"
                      className="size-5 min-w-5 p-0 opacity-0 group-hover:opacity-100 transition-opacity"
                      onPress={(e) => {
                        (e as any).stopPropagation?.();
                        removeRecentSearch(s);
                      }}
                    >
                      <IconMaterialSymbolsClose className="size-3" />
                    </Button>
                  </div>
                ))}
              </div>
            </section>
          )}

          <section className="flex flex-col gap-4">
            <Controller
              name="query"
              control={control}
              render={({ field, fieldState: { error } }) => (
                <Input
                  size="lg"
                  labelPlacement="outside"
                  label="Palabras clave"
                  placeholder="Nombre del archivo o expresión regular…"
                  autoComplete="off"
                  autoFocus
                  isClearable
                  onClear={() => field.onChange("")}
                  variant="bordered"
                  isInvalid={!!error}
                  errorMessage={error?.message}
                  classNames={{
                    label: "text-sm font-medium text-on-surface-variant",
                  }}
                  {...field}
                />
              )}
            />
            <Controller
              control={control}
              name="searchType"
              render={({ field }) => (
                <RadioGroup
                  classNames={{
                    wrapper: "gap-6",
                    label: "hidden",
                  }}
                  orientation="horizontal"
                  {...field}
                >
                  {searchTypes.map((type) => (
                    <Radio
                      value={type}
                      key={type}
                      classNames={{
                        label: "capitalize text-sm",
                      }}
                    >
                      {type === "text" ? "Coincidencia de texto" : "Expresión regular"}
                    </Radio>
                  ))}
                </RadioGroup>
              )}
            />
          </section>

          <section className="flex flex-col gap-3">
            <h3 className="text-sm font-medium text-on-surface-variant">Categorías</h3>
            <Controller
              control={control}
              name="category"
              render={({ field }) => (
                <CheckboxGroup
                  classNames={{
                    wrapper: "flex flex-wrap gap-2",
                  }}
                  orientation="horizontal"
                  {...field}
                >
                  {categories.map((category) => (
                    <FilterChip
                      startIcon={<category.icon className="size-5 max-h-none" />}
                      value={category.value}
                      key={category.value}
                    >
                      {categoryLabels[category.value] || category.value}
                    </FilterChip>
                  ))}
                </CheckboxGroup>
              )}
            />
          </section>

          <section className="flex flex-col gap-3">
            <h3 className="text-sm font-medium text-on-surface-variant">Ubicación</h3>
            <div className="flex flex-col gap-4">
              <div className="flex items-center justify-between">
                <Controller
                  control={control}
                  name="location"
                  render={({ field }) => (
                    <RadioGroup
                      classNames={{
                        wrapper: "gap-6",
                        label: "hidden",
                      }}
                      orientation="horizontal"
                      {...field}
                    >
                      {locations.map((loc) => (
                        <Radio
                          value={loc}
                          key={loc}
                          classNames={{
                            label: "capitalize text-sm",
                          }}
                        >
                          {loc === "current" ? "Carpeta actual" : "Otra ubicación"}
                        </Radio>
                      ))}
                    </RadioGroup>
                  )}
                />
                <Controller
                  name="deepSearch"
                  control={control}
                  render={({ field }) => (
                    <Checkbox
                      onChange={field.onChange}
                      isSelected={field.value}
                      className="text-sm"
                      name={field.name}
                    >
                      Incluir subcarpetas
                    </Checkbox>
                  )}
                />
              </div>

              <AnimatePresence>
                {location === "custom" && (
                  <motion.div
                    initial={{ height: 0, opacity: 0 }}
                    animate={{ height: "auto", opacity: 1 }}
                    exit={{ height: 0, opacity: 0 }}
                    className="overflow-hidden"
                  >
                    <Controller
                      name="path"
                      control={control}
                      render={({ field, fieldState: { error } }) => (
                        <Input
                          size="md"
                          placeholder="Ruta específica (p. ej., /Películas)"
                          autoComplete="off"
                          isClearable
                          onClear={() => field.onChange("")}
                          variant="bordered"
                          isInvalid={!!error}
                          errorMessage={error?.message}
                          {...field}
                        />
                      )}
                    />
                  </motion.div>
                )}
              </AnimatePresence>
            </div>
          </section>

          <section className="flex flex-col gap-3">
            <h3 className="text-sm font-medium text-on-surface-variant">Fecha de modificación</h3>
            <Controller
              control={control}
              name="modifiedDate"
              render={({ field }) => (
                <RadioGroup
                  classNames={{
                    wrapper: "flex flex-wrap gap-4",
                    label: "hidden",
                  }}
                  {...field}
                >
                  {modifiedDateValues.map((date) => (
                    <Radio
                      classNames={{
                        label: "text-sm",
                      }}
                      value={date.value}
                      key={date.value}
                    >
                      {date.label}
                    </Radio>
                  ))}
                </RadioGroup>
              )}
            />
            <AnimatePresence>
              {modifiedDate === "-2" && (
                <motion.div
                  initial={{ height: 0, opacity: 0 }}
                  animate={{ height: "auto", opacity: 1 }}
                  exit={{ height: 0, opacity: 0 }}
                  className="overflow-hidden"
                >
                  <div className="flex gap-4 pt-2">
                    <Controller
                      name="fromDate"
                      control={control}
                      render={({ field, fieldState: { error } }) => (
                        <Input
                          size="sm"
                          label="Desde"
                          type="date"
                          max={today}
                          variant="bordered"
                          isInvalid={!!error}
                          {...field}
                        />
                      )}
                    />
                    <Controller
                      name="toDate"
                      control={control}
                      render={({ field, fieldState: { error } }) => (
                        <Input
                          size="sm"
                          label="Hasta"
                          type="date"
                          max={today}
                          min={fromDate}
                          variant="bordered"
                          isInvalid={!!error}
                          {...field}
                        />
                      )}
                    />
                  </div>
                </motion.div>
              )}
            </AnimatePresence>
          </section>
        </form>

        <div className="flex gap-3 justify-end items-center p-4 border-t border-outline-variant/30 bg-surface-container-highest rounded-b-2xl">
          <Button
            onPress={() => reset(defaultFilters)}
            size="md"
            variant="text"
            className="text-on-surface-variant hover:text-on-surface"
          >
            Limpiar todo
          </Button>
          <Button
            form="filter-form"
            size="md"
            type="submit"
            isLoading={isSearching}
            variant="filled"
            className="px-8 shadow-md"
          >
            Buscar
          </Button>
        </div>
      </div>
    </Popover>
  );
});
