# Teldrive v2.0.0-drive-es.5 en español

Descargas: [GitLab](https://gitlab.com/ridrogo/teldrive/-/releases/v2.0.0-drive-es.5) · [GitHub](https://github.com/webr0m/teldrive/releases/tag/v2.0.0-drive-es.5).

Windows Desktop **2.0.0.5** añade el botón superior **Teldrive**, corrige la capa blanca de Actividad y finaliza correctamente al cerrar con los servicios detenidos.

- Importación y edición gráfica de **rclone.conf**, con respaldo, conservación de otras conexiones/comentarios y del cifrado. Las tareas pueden usar una conexión personalizada.
- Selección de **pistas de audio y subtítulos** en MKV y otros contenedores. Subtítulos de texto mediante WebVTT y de imagen incrustados mediante FFmpeg. El cambio de pistas conserva la posición del vídeo.
- **Transmitir** con Google Cast, AirPlay y DLNA. Chromecast se abre en Chrome desde Desktop; AirPlay requiere Safari en un dispositivo Apple. DLNA permite descubrir receptores en la red del servidor y reproducir/pausar/detener.
- Relay temporal de reproducción en la red local, limitado a enlaces autorizados con vencimiento. El receptor necesita una dirección accesible, no localhost. Puede requerir permitir el puerto en el firewall.
- Almacenamiento después de Papelera; icono transparente unificado en UI y CLI Windows. Desktop incluye **FFmpeg/FFprobe 9.0.2** y mantiene rclone, PostgreSQL, mpv, WinFsp y componentes Microsoft.
- Se recompilan los seis CLI: Windows, Linux y macOS, x64 y ARM64. Se conserva la corrección del HTTP 500 al abrir archivos migrados desde v1.

**Actualizar:** cierra la versión anterior y conserva config.toml, claves y carpeta de datos. Sustituye el ejecutable. No regeneres claves ni repitas la migración. En Desktop conserva `%LOCALAPPDATA%\TeldriveV2`; en modo portátil conserva `TeldriveData`.

**Verificación:** Go y API PostgreSQL 15; MKV real con dos audios y subtítulos, selección de audio/conversión/WebVTT; DLNA y relay simulados; UI en Chromium de escritorio/móvil y WebKit iPhone; WPF nativo con importación, edición, cifrado y transferencia rclone real. CLI Windows x64 ejecutado y seis arquitecturas/metadatos comprobados. Desktop incluye el mismo servidor que CLI Windows x64.

No se probaron televisores físicos. La compatibilidad depende del receptor, códecs y red. macOS y ARM64 se compilaron y se verificaron sin ejecución nativa. En CLI configura FFmpeg/FFprobe por separado para las funciones de pistas/conversión. Android/iOS conservan la edición .2 y utilizan la UI actualizada del servidor; no se recompilan APK ni IPA en esta release.


`teldrive/` contiene servidor, UI, Desktop y proyectos móviles; `rclone/` contiene el cliente adaptado. `source-revisions.json` registra los commits originales. Los blobs y permisos de las fuentes se conservan. Consulta LEEME-V2.md, VALIDACION.json y SHA256SUMS.txt en la release.
