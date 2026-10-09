export type FieldType =
  | "text"
  | "number"
  | "email"
  | "url"
  | "select"
  | "switch"
  | "textarea";

type SettingKeys =
  | "uploadConcurrency"
  | "uploadRetries"
  | "uploadRetryDelay"
  | "randomChunking"
  | "resizerHost"
  | "pageSize"
  | "splitFileSize"
  | "encryptFiles"
  | "storageLimitGB"
  | "rcloneProxy";

type SettingValue = string | number | boolean;

export interface SettingFieldConfig<T> {
  key: SettingKeys;
  type: FieldType;
  label: string;
  description: string;
  placeholder?: string;
  defaultValue?: T;
  options?: Array<{ value: T; label: string }>;
  validation?: {
    pattern?: RegExp;
    custom?: (value: SettingValue) => string | true;
  };
  category: "upload" | "display" | "security" | "other";
}

const splitFileSizes = [
  { value: 100 * 1024 * 1024, label: "100MB" },
  { value: 500 * 1024 * 1024, label: "500MB" },
  { value: 1000 * 1024 * 1024, label: "1GB" },
  { value: 2 * 1000 * 1024 * 1024, label: "2GB" },
];

export const generalSettingsConfig: SettingFieldConfig<SettingValue>[] = [
  {
    key: "storageLimitGB", type: "number", label: "Límite de almacenamiento (GB)",
    description: "Referencia personal para calcular el espacio disponible. No es una cuota de Telegram. Usa 0 para no definir un límite.",
    defaultValue: 0, category: "display",
    validation: { custom: (value) => Number.isFinite(Number(value)) && Number(value) >= 0 ? true : "Introduce un número mayor o igual a cero" },
  },
  {
    key: "uploadConcurrency",
    type: "number",
    label: "Cargas simultáneas",
    description: "Número de partes que se suben simultáneamente",
    defaultValue: 4,
    category: "upload",
  },
  {
    key: "uploadRetries",
    type: "number",
    label: "Reintentos de carga",
    description: "Número de reintentos para subir cada parte",
    defaultValue: 3,
    category: "upload",
  },
  {
    key: "uploadRetryDelay",
    type: "number",
    label: "Espera entre reintentos",
    description: "Tiempo entre reintentos, en milisegundos",
    defaultValue: 1000,
    category: "upload",
  },
  {
    key: "resizerHost",
    type: "url",
    label: "Servidor de miniaturas",
    description: "Servidor para redimensionar imágenes",
    placeholder: "https://resizer.example.com",
    category: "other",
  },
  {
    key: "pageSize",
    type: "number",
    label: "Elementos por página",
    description: "Número de elementos que se muestran por página",
    defaultValue: 500,
    category: "display",
  },
  {
    key: "splitFileSize",
    type: "select",
    label: "Tamaño de las partes",
    description: "Tamaño de cada parte al dividir archivos para subirlos",
    options: splitFileSizes,
    defaultValue: splitFileSizes[1].value,
    category: "upload",
  },
  {
    key: "encryptFiles",
    type: "switch",
    label: "Cifrar archivos",
    description: "Cifrar los archivos antes de subirlos",
    defaultValue: false,
    category: "upload",
  },
  {
    key: "randomChunking",
    type: "switch",
    label: "Nombres aleatorios de partes",
    description: "Usar nombres aleatorios para las partes de los archivos",
    defaultValue: true,
    category: "upload",
  },
  {
    key: "rcloneProxy",
    type: "url",
    label: "Proxy multimedia de Rclone",
    description: "Reproducir archivos directamente desde WebDAV de Rclone",
    placeholder: "http://localhost:8080",
    category: "other",
  },
];

type LiteralToPrimitive<T> = T extends boolean
  ? boolean
  : T extends number
    ? number
    : T extends string
      ? string
      : T;

export type Settings = {
  [P in (typeof generalSettingsConfig)[number] as P["key"]]: LiteralToPrimitive<
    P["defaultValue"]
  >;
};

export function getSettingsValues(): Settings {
  const settings = {} as any;
  for (const item of generalSettingsConfig) {
    if (item.defaultValue !== undefined) {
      settings[item.key] = item.defaultValue;
    } else {
      switch (item.type) {
        case "number":
          settings[item.key] = 0;
          break;
        case "switch":
          settings[item.key] = false;
          break;
        default:
          settings[item.key] = "";
      }
    }
  }
  return settings;
}

export const categoryConfig = {
  upload: {
    title: "Cargas",
    description: "Configura cómo se suben los archivos",
  },
  display: {
    title: "Visualización",
    description: "Personaliza cómo se muestra el contenido",
  },
  security: {
    title: "Seguridad",
    description: "Configura las opciones de seguridad",
  },
  other: {
    title: "Otras opciones",
    description: "Opciones adicionales",
  },
} as const;
