import { Button, Dropdown, Label } from "@heroui/react";
import { useAppLocale } from "@/lib/locale";
import ArrowDownIcon from "~icons/gravity-ui/arrow-down";
import ArrowUpIcon from "~icons/gravity-ui/arrow-up";
import type { FileSort, SortPreference } from "./sort-preference";

export function FileSortControls({
  sorting,
  onChange,
}: {
  sorting: SortPreference;
  onChange: (sorting: SortPreference) => void;
}) {
  const { t } = useAppLocale();
  return (
    <>
      <Dropdown>
        <Button size="sm" variant="secondary" aria-label={t("manager.sortFiles")}>
          {sorting.sort === "updatedAt"
            ? t("manager.sortModified")
            : sorting.sort === "size"
              ? t("search.sortSize")
              : t("search.sortName")}
          {sorting.order === "asc" ? " ↑" : " ↓"}
        </Button>
        <Dropdown.Popover>
          <Dropdown.Menu
            aria-label={t("manager.sortBy")}
            selectionMode="single"
            selectedKeys={new Set([sorting.sort])}
            onAction={(key) => onChange({ ...sorting, sort: key as FileSort })}
          >
            <Dropdown.Item id="name" textValue={t("search.sortName")}>
              <Label>{t("search.sortName")}</Label>
            </Dropdown.Item>
            <Dropdown.Item id="updatedAt" textValue={t("manager.sortModified")}>
              <Label>{t("manager.sortModified")}</Label>
            </Dropdown.Item>
            <Dropdown.Item id="size" textValue={t("search.sortSize")}>
              <Label>{t("search.sortSize")}</Label>
            </Dropdown.Item>
          </Dropdown.Menu>
        </Dropdown.Popover>
      </Dropdown>
      <Button
        isIconOnly
        size="sm"
        variant="secondary"
        aria-label={t(sorting.order === "asc" ? "manager.orderToDesc" : "manager.orderToAsc")}
        onPress={() => onChange({ ...sorting, order: sorting.order === "asc" ? "desc" : "asc" })}
      >
        {sorting.order === "asc" ? (
          <ArrowUpIcon className="size-4" aria-hidden="true" />
        ) : (
          <ArrowDownIcon className="size-4" aria-hidden="true" />
        )}
      </Button>
    </>
  );
}
