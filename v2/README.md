# Teldrive v2.0.0-drive-es.6

[GitLab](https://gitlab.com/ridrogo/teldrive/-/releases/v2.0.0-drive-es.6) · [GitHub](https://github.com/webr0m/teldrive/releases/tag/v2.0.0-drive-es.6).

Actualiza la UI integrada de los seis CLI y Windows Desktop **2.0.0.6**.

- **Móvil, modo claro:** menú opaco, campos y barras del visor legibles. Los marcadores antiguos redirigen a las pantallas actuales. El cambio de tema conserva la navegación.
- **Audio automático:** se elige la pista predeterminada declarada en el contenedor, aunque no sea la primera. MKV y audio no compatible utilizan conversión automática cuando FFmpeg está disponible. Se mantiene la selección manual de pistas y subtítulos y la transmisión.
- **Montajes persistentes:** en Opciones adicionales de rclone → Tareas elige «Montar unidad persistente», conexión, carpeta y letra libre. Configura caché/solo lectura y marca el montaje al iniciar. Activa el inicio con Windows en Preferencias. Espera al servidor y al desbloqueo de rclone.conf; no guarda contraseñas. Un montaje no bloquea transferencias. Cancelar desmonta; el siguiente inicio recupera el montaje. Una cola pausada debe reanudarse.
- **Diez idiomas sin conexión:** español, inglés, portugués, francés, alemán, italiano, ruso, chino simplificado, japonés y árabe. Selector en Apariencia/inicio de sesión web y Preferencias de Desktop; elección persistente. Árabe utiliza lectura de derecha a izquierda. Se conservan nombres, rutas y credenciales. Traducciones iniciales automáticas, revisables en localization/catalog.json. La ayuda técnica del terminal mantiene su idioma original.
- **Doble clic de Desktop:** elimina el evento adicional que enviaba WebView2, y vuelve a abrir carpetas/archivos con doble clic en lista y cuadrícula. Se conserva Actividad sin la superposición blanca.

**Verificado:** ejecutable final en WPF/WebView2, doble clic nativo, diez idiomas con bindings WPF, montaje WinFsp real, espera del servidor/desbloqueo, reinicio, transferencia concurrente y desmontaje. UI en Chromium de escritorio/móvil y WebKit iPhone, con renombrado/navegación en diez idiomas. Go completo y MKV real: la conversión predeterminada reproduce la segunda pista marcada por el contenedor, comprobada por su frecuencia de audio. CLI Windows x64 ejecutado; arquitecturas, metadatos, hashes y contenido de los seis paquetes comprobados. Desktop incluye exactamente el servidor CLI Windows x64 y los 22 componentes/licencias de la edición anterior.

**Actualizar:** cierra Desktop desde «Salir y detener servicios», sustituye el ejecutable y conserva config.toml, claves y carpeta de datos. No regeneres claves ni repitas la migración. Conserva %LOCALAPPDATA%\TeldriveV2 o TeldriveData en modo portátil. Si cambias el nombre/ubicación, guarda de nuevo el inicio automático.

Linux/macOS/ARM64 se compilaron y verificaron sin ejecución nativa. CLI necesita FFmpeg/FFprobe configurados para conversión/pistas. No se probaron televisores físicos. Android/iOS conservan sus clientes .2 y utilizan la UI actualizada del servidor; esta release no recompila APK ni IPA. Fuentes, instrucciones y SHA256 incluidos.

Fuentes en teldrive/ y rclone/. source-revisions.json conserva los commits originales; blobs y permisos se preservan.
