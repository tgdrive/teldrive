// Same order as the fields exported from config.ServerCmdConfig.
export const cliLabels = [
  "Puerto HTTP", "Tiempo de apagado ordenado", "Depuración pprof", "Tiempo máximo de lectura", "Tiempo máximo de respuesta",
  "Dirección de escucha HTTP",
  "Ruta del ejecutable FFmpeg", "Conversiones multimedia simultáneas", "Duración máxima de conversión multimedia",
  "Nivel de registros", "Formato de fecha del registro", "Archivo de registros", "Registrar peticiones HTTP", "Registrar consultas completas",
  "Ocultar parámetros sensibles", "Longitud máxima de consulta", "Registrar agente del navegador", "Registrar tamaño de petición", "Registrar tamaño de respuesta", "Rutas excluidas del registro",
  "Nivel de registros de la base de datos", "Umbral de consultas lentas", "Omitir errores de registro inexistente", "Registrar consultas SQL", "Registros internos de Telegram", "Nivel de registros de Telegram",
  "Secreto de firma JWT", "Duración de la sesión", "Usuarios permitidos", "Conexión PostgreSQL", "Consultas preparadas", "Agrupar conexiones", "Máximo de conexiones abiertas", "Máximo de conexiones inactivas", "Duración máxima de conexión",
  "Limitar peticiones a Telegram", "Ráfaga máxima de peticiones", "Peticiones por minuto", "Sincronizar hora mediante NTP", "Proxy HTTP o SOCKS5", "Tiempo de reconexión", "Tamaño del grupo de sesiones", "Registro antiguo de Telegram (obsoleto)", "ID de aplicación de Telegram", "Hash de aplicación de Telegram", "Modelo del dispositivo", "Versión del sistema", "Versión de la aplicación", "Código de idioma", "Idioma del sistema", "Paquete de idioma", "Nombre de instancia de sesiones", "Crear canales automáticamente", "Límite de mensajes por canal",
  "Clave de cifrado de las subidas", "Hilos de subida", "Reintentos de subida", "Retención de subidas incompletas", "Hilos de reproducción", "Búferes de reproducción", "Tiempo máximo por fragmento", "Límite de bots de reproducción (0: todos)", "Almacenamiento de sesiones", "Prefijo de sesiones", "Archivo BoltDB", "Tiempo máximo de apertura de BoltDB", "Desactivar sincronización al ampliar BoltDB",
  "Activar tareas programadas", "Nombre del bloqueo de tareas", "Intervalo de limpieza de archivos", "Intervalo de limpieza de subidas", "Intervalo de cálculo de carpetas", "Tamaño máximo de caché en bytes",
  "Dirección de Redis", "Contraseña de Redis", "Tamaño del grupo Redis", "Mínimo de conexiones Redis inactivas", "Máximo de conexiones Redis inactivas", "Tiempo máximo inactivo en Redis", "Duración máxima de conexión Redis",
  "Intervalo de consulta de eventos", "Procesos de persistencia de eventos", "Tamaño de cola de eventos", "Duración de deduplicación de eventos",
]

export const cliGroups: Record<string, string> = { server: "Servidor", log: "Registros", jwt: "Acceso y sesiones", db: "Base de datos", tg: "Telegram y multimedia", "cron-jobs": "Tareas programadas", cache: "Caché", redis: "Redis", events: "Eventos" }
