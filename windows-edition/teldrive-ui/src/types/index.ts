import type { operations } from "@/lib/api";
import type { Dispatch, SetStateAction } from "react";

export type FileResponse = {
  files: SingleFile[];
  meta: { totalPages: number; count: number; currentPage: number };
};

export type SingleFile = {
  name: string;
  type: string;
  mimeType: string;
  size: number;
  depth: number;
  createdAt: string;
  updatedAt: string;
  userId: string;
  parentId: string;
  id: string;
  encrypted?: boolean;
  path?: string;
};

export type FilePayload = {
  id?: string;
  payload?: Record<string, any>;
};

export type UploadPart = {
  name: string;
  partId: number;
  partNo: number;
  size: number;
  channelId: number;
  encrypted?: boolean;
  salt?: string;
};

export type AuthMessage = {
  type: string;
  payload: Record<string, string | number | boolean>;
  message: string;
};

export type Message = {
  message: string;
  error?: string;
  code?: number;
};

export type Session = {
  name: string;
  userName: string;
  userId: number;
  isPremium: boolean;
  hash: string;
  expires: string;
};

export type UserSession = {
  hash: string;
  createdAt: string;
  location?: string;
  officialApp?: boolean;
  appName?: string;
  valid: boolean;
  current: boolean;
};

export type BrowseView = "my-drive" | "search" | "recent" | "browse" | "shared";

export type FileListParams = {
  view: BrowseView;
  params: Exclude<operations["Files_list"]["parameters"]["query"], undefined>;
};

export type ShareListParams = {
  id: string;
  path?: string;
};

export type AccountStats = {
  channelId: number;
  bots: string[];
};

export type Channel = {
  channelName?: string;
  channelId: number;
};

export type Tags = {
  [key: string]: any;
};

export type AudioMetadata = {
  artist: string;
  title: string;
  cover: string;
};

export type UploadStats = {
  uploadDate: string;
  totalUploaded: number;
};

export type CategoryStorage = {
  category: string;
  totalFiles: number;
  totalSize: number;
};

export type FileShare = {
  id: string;
  expirationDate: string;
  protected: boolean;
  type: string;
  name: string;
};

export type SetValue<T> = Dispatch<SetStateAction<T>>;

export type PreviewFile = {
  id: string;
  name: string;
  mimeType: string;
  previewType: string;
};
