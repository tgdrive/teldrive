import type { FileUploadStatus } from "@/utils/stores";

export interface CircularProgressProps {
  progress: number;
  size?: number;
  strokeWidth?: number;
  className?: string;
  showCancel?: boolean;
  onCancel?: () => void;
  status?: FileUploadStatus;
}

export type UploadParams = Record<
  string,
  string | number | boolean | undefined
>;

export interface UploadProps {
  queryKey: any[];
}
