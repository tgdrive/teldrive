import type { SetValue, UploadStats } from "@/types";
import {
  Button,
  Dropdown,
  DropdownItem,
  DropdownMenu,
  DropdownTrigger,
} from "@tw-material/react";
import type { ApexOptions } from "apexcharts";
import { memo, useMemo } from "react";
import ReactApexChart from "react-apexcharts";

const options: ApexOptions = {
  legend: {
    show: false,
  },
  colors: ["oklch(var(--m3-primary))"],
  chart: {
    height: 250,
    type: "area",
    fontFamily: "Rubik, sans-serif",
    toolbar: {
      show: false,
    },
    zoom: {
      enabled: false,
    },
    sparkline: {
      enabled: false,
    },
  },
  fill: {
    type: "gradient",
    gradient: {
      shadeIntensity: 1,
      opacityFrom: 0.45,
      opacityTo: 0.05,
      stops: [20, 100, 100, 100],
    },
  },
  stroke: {
    width: 3,
    curve: "smooth",
  },
  grid: {
    show: true,
    borderColor: "oklch(var(--m3-outline-variant) / 0.3)",
    strokeDashArray: 4,
    padding: {
      left: 20,
      right: 20,
      bottom: 0,
    },
  },
  dataLabels: {
    enabled: false,
  },
  markers: {
    size: 5,
    colors: ["oklch(var(--m3-primary))"],
    strokeColors: "oklch(var(--m3-surface))",
    strokeWidth: 2,
    hover: {
      size: 7,
    },
  },
  xaxis: {
    type: "category",
    axisBorder: {
      show: false,
    },
    axisTicks: {
      show: false,
    },
    labels: {
      style: {
        colors: "oklch(var(--m3-on-surface-variant))",
        fontSize: "12px",
        fontWeight: 500,
      },
    },
  },
  yaxis: {
    labels: {
      style: {
        colors: "oklch(var(--m3-on-surface-variant))",
        fontSize: "12px",
        fontWeight: 500,
      },
    },
  },
  tooltip: {
    theme: "dark",
    x: {
      show: true,
    },
    y: {
      formatter: (val) => `${val.toFixed(2)} GB`,
    },
    style: {
      fontSize: "12px",
      fontFamily: "Rubik, sans-serif",
    },
  },
};

function getChartData(stats: UploadStats[]): ApexOptions {
  const categories = stats.map((stat) => stat.uploadDate);
  const data = stats.map((stat) => stat.totalUploaded);
  return {
    ...options,
    xaxis: {
      ...options.xaxis,
      categories,
    },
    series: [
      {
        name: "Cargado",
        data,
      },
    ],
  };
}

interface UploadStatsChartProps {
  stats: UploadStats[];
  days: number;
  setDays: SetValue<number>;
}

const allowedDays = [7, 15, 30, 60];

export const UploadStatsChart = memo(
  ({ stats, days, setDays }: UploadStatsChartProps) => {
    const chartOptions = useMemo(() => getChartData(stats), [stats]);

    return (
      <div className="w-full">
        <div className="flex justify-end mb-2">
          <Dropdown className="min-w-32" triggerScaleOnOpen={false}>
            <DropdownTrigger>
              <Button
                variant="filledTonal"
                className="rounded-xl px-4 py-2 font-medium bg-secondary-container text-on-secondary-container"
                size="sm"
              >{`${days} días`}</Button>
            </DropdownTrigger>
            <DropdownMenu
              classNames={{
                base: "bg-surface-container-high rounded-2xl shadow-2xl border border-outline-variant/30",
              }}
              itemClasses={{
                base: "rounded-xl data-[hover=true]:bg-on-surface/10 px-4 py-2 transition-colors",
                title: "text-sm font-medium",
              }}
            >
              {allowedDays.map((day) => (
                <DropdownItem key={day} onPress={() => setDays(day)}>
                  {`${day} días`}
                </DropdownItem>
              ))}
            </DropdownMenu>
          </Dropdown>
        </div>
        <div className="min-h-[250px]">
          <ReactApexChart
            options={chartOptions}
            series={chartOptions.series}
            type="area"
            height={250}
          />
        </div>
      </div>
    );
  },
);
