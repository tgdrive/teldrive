# Teldrive v2.0.0-drive-es.4 en español

Corrige el HTTP 500 al abrir o descargar archivos migrados desde v1 con tamaños pendientes y conexiones PostgreSQL en modo `exec` o protocolo simple. El JSON de tamaños se envía como texto y funciona con los cinco modos de conexión de pgx. Incluye seis servidores CLI nuevos con UI integrada y Windows Desktop 2.0.0.4.

Descargas: [GitLab](https://gitlab.com/ridrogo/teldrive/-/releases/v2.0.0-drive-es.4) · [GitHub](https://github.com/webr0m/teldrive/releases/tag/v2.0.0-drive-es.4).

Cada paquete CLI incluye el rclone compatible con la API v2: Windows, Linux y macOS, en x64 y ARM64. El cliente rclone mantiene su versión anterior; la corrección está en el servidor. Windows completo conserva configuración gráfica, tareas, cola, programador, bandeja e inicio automático opcional, multimedia e icono transparente.

Para actualizar una instalación v2, cierra la edición anterior y reemplaza el ejecutable conservando `config.toml`, claves y carpeta de datos. Esta corrección no necesita regenerar claves ni repetir la migración. Consulta `LEEME-V2.md` de la release y `teldrive/desktop/README.md`.

## Fuentes y validación

`teldrive/` contiene servidor, UI, Desktop y proyectos móviles; `rclone/` contiene el cliente adaptado. `source-revisions.json` registra los commits originales; la sincronización conserva blobs y permisos Unix. `Teldrive-v2-fuentes.zip` incluye ambos proyectos. Configuraciones personales, credenciales, bases de datos y claves privadas quedan fuera.

Pruebas unitarias de Go y catálogo con PostgreSQL; regresión de los cinco modos de pgx en PostgreSQL 15 y 18. Se ejecutaron Windows x64 y Linux x64 y se verificaron metadatos, arquitectura y contenido de los seis paquetes. Se comprobó una descarga PDF completa y lecturas parciales de PDF/vídeo con Telegram real. Desktop incluye exactamente el mismo servidor que CLI Windows x64. Los resultados están en `VALIDACION.json`, `BUILD-MANIFEST.json` y `SHA256SUMS.txt`.

macOS y ARM64 están compilados y sus binarios verificados, sin ejecución nativa en esos equipos. Android e iOS conservan la [edición .2](https://gitlab.com/ridrogo/teldrive/-/releases/v2.0.0-drive-es.2) y utilizan el servidor actualizado; iOS sigue siendo un proyecto fuente sin IPA. Las releases anteriores se conservan.
