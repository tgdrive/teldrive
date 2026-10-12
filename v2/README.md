# Teldrive v2.0.0-drive-es.7

[GitLab](https://gitlab.com/ridrogo/teldrive/-/releases/v2.0.0-drive-es.7) · [GitHub](https://github.com/webr0m/teldrive/releases/tag/v2.0.0-drive-es.7).

Actualización de los seis CLI y Windows Desktop **2.0.0.7** para generar enlaces públicos con tu dominio incluso al copiar desde localhost o Desktop.

Agrega este valor a la sección `[http]` existente de tu `config.toml`:

```toml
public-url = "https://drive.tudominio.com"
```

Sustituye el ejemplo por tu dominio y reinicia Teldrive. En Desktop edita Configuración → Configuración avanzada y guarda. Configura Cloudflare Tunnel o el proxy por separado; el ajuste no crea DNS, HTTPS ni un túnel. Conserva `address = "127.0.0.1:8080"` si cloudflared está en el mismo equipo. En ese caso puedes configurar `trusted-proxies = ["127.0.0.1/32", "::1/128"]`.

- La dirección pública se usa al crear enlaces nuevos, desde cualquier origen de la interfaz.
- Con el valor vacío se conserva la dirección del navegador.
- Valida HTTP/HTTPS, puerto y formato; rechaza credenciales, subrutas, consultas y fragmentos sin revelar posibles claves en los errores.
- Acepta también `--http-public-url` y `TELDRIVE_HTTP_PUBLIC_URL`.
- Los tokens previos siguen funcionando con el dominio público. Los enlaces ya copiados mantienen su dirección anterior.
- Conserva las funciones de .6: diez idiomas, modo claro móvil, audio automático, pistas/subtítulos, transmisión, tareas y montaje persistente de rclone.

**Comprobado:** suite Go completa; configuración mediante TOML/entorno/CLI; seis pruebas de copia de enlaces públicos/locales en Chromium escritorio, Android emulado y WebKit iPhone; CLI Windows x64 ejecutado; arquitecturas/metadatos/hashes de los seis binarios; arranque real de Desktop con PostgreSQL/WebView2 en datos de prueba aislados. Desktop integra el mismo servidor Windows x64 y los 22 componentes con licencias.

**Actualizar:** cierra la versión anterior y conserva config.toml, claves y carpeta de datos. No regeneres claves ni repitas la migración. En Desktop conserva `%LOCALAPPDATA%\TeldriveV2` o `TeldriveData` en modo portátil. Guarda otra vez el inicio automático si cambias la ruta.

Linux/macOS/ARM64 se compilaron sin ejecución nativa. No se ha configurado ni probado el dominio real del usuario. CLI requiere PostgreSQL y FFmpeg/FFprobe externos. Android/iOS conservan los clientes .2 y utilizan la UI actualizada del servidor; esta release no recompila APK o IPA.

Fuentes en teldrive/ y rclone/. source-revisions.json conserva los commits originales; blobs y permisos se preservan.
