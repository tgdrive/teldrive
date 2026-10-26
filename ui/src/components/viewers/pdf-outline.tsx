import { useOutline, useOutlineState } from "@embedpdf/react/outline";
import { useStage } from "@embedpdf/react/stage";
import { Button, cn, Spinner } from "@heroui/react";
import { useEffect } from "react";
import { useAppLocale } from "@/lib/locale";
import DownIcon from "~icons/gravity-ui/chevron-down";
import RightIcon from "~icons/gravity-ui/chevron-right";

export function PdfOutline({ onNavigate }: { onNavigate: () => void }) {
  const { t } = useAppLocale();
  const outline = useOutline();
  const stage = useStage();
  const { rows, current, status } = useOutlineState();

  useEffect(() => {
    void outline.ensureLoaded().catch(() => {});
  }, [outline]);

  if (status === "idle" || status === "loading") {
    return (
      <div role="status" className="flex justify-center p-6">
        <Spinner size="sm" aria-label={t("pdf.loadingDoc")} />
      </div>
    );
  }
  if (status === "error" || status === "forbidden") {
    return (
      <div role="alert" className="space-y-3 p-4 text-xs text-muted">
        <p>
          {t("pdf.tabOutline")}: {t("pdf.openFailed")}
        </p>
        <Button
          size="sm"
          variant="secondary"
          onPress={() => {
            void outline.ensureLoaded().catch(() => {});
          }}
        >
          {t("pdf.retry")}
        </Button>
      </div>
    );
  }
  if (!rows.length)
    return <p className="px-3 py-8 text-center text-xs text-muted">{t("pdf.noOutline")}</p>;

  return (
    <nav aria-label={t("pdf.tabOutline")} className="p-2">
      {rows.map(({ item, depth, hasChildren, expanded }) => {
        const title = item.title || t("pdf.untitled");
        const target = item.target;
        const isCurrent = current?.objectNumber === item.ref.objectNumber;
        return (
          <div
            key={item.ref.objectNumber}
            className="flex items-center gap-0.5"
            style={{ paddingInlineStart: `${depth * 14}px` }}
          >
            {hasChildren ? (
              <Button
                isIconOnly
                size="sm"
                variant="ghost"
                className="size-7 shrink-0"
                aria-label={t(expanded ? "pdf.collapseSection" : "pdf.expandSection", { title })}
                aria-expanded={expanded}
                onPress={() => outline.toggle(item.ref)}
              >
                {expanded ? <DownIcon className="size-3.5" /> : <RightIcon className="size-3.5" />}
              </Button>
            ) : (
              <span className="w-7 shrink-0" aria-hidden="true" />
            )}
            <Button
              size="sm"
              variant={isCurrent ? "secondary" : "ghost"}
              aria-current={isCurrent ? "location" : undefined}
              isDisabled={target?.kind !== "goto"}
              className="h-auto min-w-0 flex-1 justify-start whitespace-normal py-2 text-left text-xs"
              onPress={() => {
                // Chapter navigation only: do not execute PDF actions or open URLs.
                if (target?.kind !== "goto") return;
                stage.goToDestination(target.destination);
                onNavigate();
              }}
            >
              <span
                className={cn(
                  "line-clamp-2",
                  item.bold && "font-semibold",
                  item.italic && "italic",
                )}
              >
                {title}
              </span>
            </Button>
          </div>
        );
      })}
    </nav>
  );
}
