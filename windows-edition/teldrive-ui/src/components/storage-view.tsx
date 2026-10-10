import type { CategoryStorage, UploadStats } from "@/types";
import { queryOptions, useSuspenseQueries } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { filesize } from "filesize";
import { memo, useState } from "react";

import { $api } from "@/utils/api";
import { bytesToGB } from "@/utils/common";
import { categoryLabels } from "@/utils/locale";
import { UploadStatsChart } from "./charts/upload-stats";

import IcOutlineArchive from "~icons/ic/outline-archive";
import IcOutlineAudiotrack from "~icons/ic/outline-audiotrack";
import IcOutlineStorage from "~icons/ic/outline-cloud";
import IcOutlineDataset from "~icons/ic/outline-dataset";
import IcOutlineDescription from "~icons/ic/outline-description";
import IcOutlineFolder from "~icons/ic/outline-folder";
import IcOutlineImage from "~icons/ic/outline-image";
import IcOutlineInsertDriveFile from "~icons/ic/outline-insert-drive-file";
import IcOutlineVideocam from "~icons/ic/outline-videocam";

const categoryIcons: Record<string, React.ElementType> = {
  video: IcOutlineVideocam,
  audio: IcOutlineAudiotrack,
  image: IcOutlineImage,
  document: IcOutlineDescription,
  folder: IcOutlineFolder,
  archive: IcOutlineArchive,
  other: IcOutlineInsertDriveFile,
};

function getTotalStats(data: CategoryStorage[]) {
  return data.reduce(
    (acc, item) => {
      acc.totalSize += item.totalSize;
      acc.totalFiles += item.totalFiles;
      return acc;
    },
    { totalSize: 0, totalFiles: 0 },
  );
}

const CategoryStorageCard = memo(
  ({
    category,
    totalSize,
    totalFiles,
    allTotalSize,
  }: CategoryStorage & { allTotalSize: number }) => {
    const Icon = categoryIcons[category] || IcOutlineInsertDriveFile;
    const percentage = allTotalSize > 0 ? (totalSize / allTotalSize) * 100 : 0;

    return (
      <Link
        to="/$view"
        params={{ view: "browse" }}
        search={category === "folder" ? { type: "folder" } : { category: [category] as any }}
        className="group relative flex flex-col gap-4 p-5 bg-surface-container-low border border-outline-variant/50 rounded-[28px] hover:bg-surface-container transition-all duration-300"
      >
        <div className="flex justify-between items-start">
          <div className="p-3 rounded-2xl bg-secondary-container text-on-secondary-container transition-transform group-hover:scale-110 duration-300">
            <Icon className="size-6" />
          </div>
          <div className="text-right">
            <p className="text-sm font-medium text-on-surface-variant">{categoryLabels[category] || category}</p>
            <p className="text-lg font-bold text-on-surface">{totalFiles} Archivos</p>
          </div>
        </div>

        <div className="mt-2">
          <p className="text-2xl font-bold text-on-surface">
            {filesize(totalSize, { standard: "jedec" })}
          </p>
          <div className="mt-3 h-2 w-full bg-surface-container-high rounded-full overflow-hidden">
            <div
              className="h-full bg-primary transition-all duration-1000 ease-out"
              style={{ width: `${percentage}%` }}
            />
          </div>
        </div>
      </Link>
    );
  },
);

export const StorageView = memo(() => {
  const [days, setDays] = useState(7);
  const [{ data: uploadStats }, { data: categories }] = useSuspenseQueries({
    queries: [
      queryOptions({
        ...$api.queryOptions("get", "/uploads/stats", {
          params: {
            query: {
              days: 60,
            },
          },
        }),
        select: (data) =>
          data.slice(data.length - days, data.length).map((stat) => {
            const options = { day: "numeric", month: "short" } as const;
            const formattedDate = new Intl.DateTimeFormat("es-CL", options).format(
              new Date(stat.uploadDate),
            );
            return {
              totalUploaded: bytesToGB(stat.totalUploaded),
              uploadDate: formattedDate,
            };
          }),
      }),
      queryOptions({
        ...$api.queryOptions("get", "/files/categories"),
        select: (data) => ({
          data: data as CategoryStorage[],
          totalStats: getTotalStats(data as CategoryStorage[]),
        }),
      }),
    ],
  });

  const categoryData = (categories as any).data as CategoryStorage[];
  const totalStats = (categories as any).totalStats as { totalSize: number; totalFiles: number };

  return (
    <div className="flex flex-col gap-6">
      <div className="bg-surface-container-low rounded-[32px] p-6 md:p-8 border border-outline-variant/50">
        <div className="flex flex-col md:flex-row justify-between items-start md:items-center gap-6">
          <div className="flex items-center gap-4">
            <div className="p-4 rounded-[24px] bg-primary text-on-primary">
              <IcOutlineStorage className="size-8" />
            </div>
            <div>
              <h2 className="text-2xl md:text-3xl font-bold text-on-surface">Almacenamiento total</h2>
              <p className="text-on-surface-variant font-medium">Resumen del uso de tu unidad</p>
            </div>
          </div>
          <div className="flex gap-8 md:gap-12">
            <div className="flex flex-col">
              <span className="text-sm font-medium text-on-surface-variant">Espacio utilizado</span>
              <span className="text-3xl font-bold text-primary">
                {filesize(totalStats.totalSize, { standard: "jedec" })}
              </span>
            </div>
            <div className="flex flex-col">
              <span className="text-sm font-medium text-on-surface-variant">Total de archivos</span>
              <span className="text-3xl font-bold text-primary">{totalStats.totalFiles}</span>
            </div>
          </div>
        </div>
      </div>

      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
        {categoryData.map((category) => (
          <CategoryStorageCard
            key={category.category}
            {...category}
            allTotalSize={totalStats.totalSize}
          />
        ))}
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-12 gap-6">
        <div className="lg:col-span-12 bg-surface-container-low rounded-[32px] p-6 border border-outline-variant/50">
          <div className="flex items-center gap-3 mb-6">
            <div className="p-2 rounded-xl bg-secondary-container text-on-secondary-container">
              <IcOutlineDataset className="size-6" />
            </div>
            <h3 className="text-xl font-bold text-on-surface">Actividad de carga</h3>
          </div>
          <UploadStatsChart days={days} setDays={setDays} stats={uploadStats as any} />
        </div>
      </div>
    </div>
  );
});
