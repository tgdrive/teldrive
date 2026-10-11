# Teldrive v2.0.0-drive-es.2 en español

Actualización de la edición v2 con ordenación persistente, correcciones del tema automático, notificaciones adaptadas al tema, importación de preferencias anteriores y mejoras de bots/canales. Conserva las paletas configurables, almacenamiento, papelera, Spam, multimedia y controles táctiles en español.

Descargas: [GitHub](https://github.com/webr0m/teldrive/releases/tag/v2.0.0-drive-es.2) · [GitLab](https://gitlab.com/ridrogo/teldrive/-/releases/v2.0.0-drive-es.2).

## Fuentes

- `teldrive/`: backend, UI, Windows Desktop .NET/WPF y proyectos Android/iOS.
- `rclone/`: cliente compatible con la API v2; mantiene la revisión comprobada de la edición anterior.
- `source-revisions.json`: revisiones exactas, verificadas junto con los blobs y permisos Unix de todos los archivos.
- `teldrive/LEEME-V2.md`: configuración, actualización y compilación.

Los componentes grandes se distribuyen en las releases. El ZIP de fuentes y el checkout conservan los permisos de los scripts. Las claves privadas de firma, configuraciones personales y bases de datos quedan fuera del repositorio.

## Comprobaciones y límites

La sincronización pasó comprobación de tipos y compilación de la UI, pruebas unitarias y vet de Go, regeneración SQLC con su parche de esquema, pruebas PostgreSQL de canales/bots y 18 pruebas de escritorio más 10 de controles móviles en Chromium/WebKit. La nueva publicación recompila seis servidores CLI, Windows Desktop y el APK firmado; los resultados finales y hashes están en `VALIDACION.json`, `BUILD-MANIFEST.json` y `SHA256SUMS.txt` de la release.

Android conserva la clave de firma y aumenta su versión a 200002. iOS se ofrece como proyecto fuente sin IPA compilada ni firmada. No se han probado teléfonos físicos ni equipos macOS/ARM64 ni una cuenta real de Telegram. El montaje macOS ARM64 sigue pendiente de una compilación nativa con CGO/macFUSE. Las releases anteriores se conservan con sus archivos y manifiestos originales.
