# Teldrive v2.0.0-drive-es.8

Edición basada en la rama v2 de tgdrive/teldrive, con interfaz en español, temas configurables, navegación táctil, almacenamiento, papelera y Spam. La UI se incluye en los seis servidores CLI para Windows, Linux y macOS, en x64 y ARM64.

## Cambios de esta actualización

**Configuración → Bots** permite crear bots con tu sesión de Telegram y un bot
gestor. Importa una vez el token del gestor, activa **Bot Management Mode** en
BotFather, selecciónalo y verifícalo. Introduce nombre y usuario, comprueba la
disponibilidad y pulsa **Crear y añadir bot**. El token del nuevo bot se obtiene
automáticamente, se guarda cifrado y se programa la incorporación a tus canales.
Si se interrumpe, repite el mismo usuario y gestor para recuperar un bot propio.
La importación de tokens existentes sigue disponible. Requiere Telegram remoto.
Consulta `desktop/README.md` para los pasos completos. Conserva las mejoras de
las ediciones anteriores descritas a continuación.

Desktop añade un botón superior **Teldrive** para recuperar el Drive desde cualquier panel y corrige la capa de WebView2 que tapaba Actividad. **Opciones adicionales de rclone → rclone.conf** permite importar, exportar y editar conexiones con campos privados ocultos, respaldos y conservación del cifrado. Las tareas permiten elegir una conexión de un archivo personalizado. La UI de los CLI también importa, edita y exporta rclone.conf desde el navegador.

Almacenamiento aparece al final, después de Papelera. La UI, los favicons y los ejecutables CLI Windows usan el icono transparente del avión.

El reproductor añade **Transmitir**: Chromecast/Google Cast desde Chrome, AirPlay desde Safari y búsqueda/control de televisores DLNA desde una cuenta administradora. En Windows Desktop, Chromecast abre un reproductor con enlace temporal en Chrome; AirPlay puede utilizarse abriendo el enlace/QR en Safari de un dispositivo Apple. La conversión compatible aplica H.264/AAC y las pistas seleccionadas. El servidor puede habilitar un enlace limitado al vídeo en la red local al pedir Transmitir, sin cambiar la dirección de la UI principal; el enlace vence y respeta la revocación de acceso. Televisor y servidor deben poder comunicarse, y el cortafuegos debe permitir la conexión en la red privada.

MKV y otros contenedores muestran las pistas detectadas por FFprobe: audio, idiomas, títulos y subtítulos. Los subtítulos de texto se ofrecen como WebVTT; los de imagen, como PGS, se integran en la conversión. Desktop incluye FFmpeg/FFprobe 9.0.2; los CLI requieren estas herramientas instaladas junto al ejecutable configurado de FFmpeg. No todos los formatos propietarios, archivos dañados o DRM admiten conversión.


Integra las correcciones recientes de la rama original v2: ordenación de archivos persistente, modo claro/oscuro/automático conservado al reiniciar, notificaciones que siguen el tema, mejoras en bots/canales y documentación actualizada. Conserva los temas personalizados y los controles en español. Las preferencias del tema anterior se importan automáticamente. El lanzador de pruebas de la UI funciona en Windows y Unix.

Los binarios de esta edición se recompilan desde estas fuentes. Android mantiene la misma clave de firma y aumenta su código de versión para instalarse como actualización. Antes de actualizar Windows, cierra la aplicación y conserva una copia de tu carpeta de datos; sustituye el ejecutable, sin borrar la configuración ni la base de datos. Las migraciones se aplican al iniciar el servidor.

## Windows completo

La actualización **Windows Desktop 2.0.0.3** añade tareas guardadas, cola de transferencias, programador y preferencias de bandeja en **Opciones adicionales de rclone**, junto con el nuevo icono del avión con fondo transparente. Las transferencias del explorador utilizan la cola. Esta actualización se concentra en Windows portátil; las descargas CLI, Android e iOS de la publicación `v2.0.0-drive-es.2` continúan disponibles sin sustituirlas. Los horarios necesitan el programa abierto, incluso oculto en la bandeja. Consulta `desktop/README.md` para las opciones y validación.

Abre `Teldrive Desktop v2.exe`. La ventana de .NET 10/WPF presenta la configuración inicial y después la UI mediante WebView2. Incluye Teldrive, rclone compatible con v2, PostgreSQL, FFmpeg, mpv y los instaladores oficiales de WinFsp, WebView2 y Visual C++. No necesitas instalar .NET por separado. WinFsp requiere el permiso de administrador de Windows para instalar su controlador; solo es necesario para montar unidades.

En **Configuración → Rclone** puedes guardar y cifrar la conexión, explorar archivos locales y remotos, subir, descargar, renombrar, crear carpetas, revisar y reintentar transferencias, montar una unidad y reproducir multimedia con mpv. Consulta `desktop/README.md` para el modo portátil y los detalles.

La base de datos de esta edición se guarda por separado de la edición 1.8.3. Haz una copia antes de migrar una base existente. El primer inicio requiere tu configuración de Telegram; las pruebas locales no verifican una cuenta real de Telegram.

## CLI y rclone

El paquete de cada plataforma contiene el servidor y su UI, más el cliente rclone adaptado a la API v2. Copia `config.sample.toml` a `config.toml`, configura `[database] url`, Telegram y tus claves privadas de `[security]`, y ejecuta:

```sh
teldrive check --config config.toml
teldrive run --config config.toml
```

En Windows usa `teldrive.exe`. Abre la dirección de `[http] address`, por defecto `http://127.0.0.1:8080`. PostgreSQL se configura por separado en el modo CLI. La conversión multimedia requiere FFmpeg con libx264 y libmp3lame disponible en `[media] ffmpeg-path`; Windows Desktop ya lo incluye. Usa `teldrive --help` y `rclone help` para las opciones completas.

Los rclone de macOS permiten transferencias. El montaje nativo de macOS ARM64 no está incluido; las compilaciones actuales se generaron sin CGO. No se ha probado su ejecución en equipos macOS. Windows x64 sí se ha probado con WinFsp.

## Compartir, papelera y Spam

Los enlaces públicos admiten contraseña, caducidad y límites. La nueva vista previa reserva una descarga por sesión, conservándola al buscar posiciones o cambiar a reproducción compatible. La contraseña se envía en una cabecera, sin añadirla a la dirección del reproductor. La revocación se comprueba en cada nueva petición.

Papelera y Spam incluyen filtros de tipo y modificación, lista y cuadrícula. Los elementos propios se conservan durante 30 días, salvo eliminación definitiva manual. Marcar un elemento propio como Spam revoca sus enlaces y permisos; restaurarlo no los vuelve a conceder. Marcar un elemento recibido oculta y bloquea tu acceso, sin borrar el original ni afectar a otros destinatarios. Retirar acceso o llegar a 30 días revoca solo tu permiso.

La barra muestra el uso registrado por Teldrive. El límite configurable es una referencia personal; Telegram no publica una cuota equivalente a Google Drive. La reproducción compatible convierte audio a MP3 y vídeo a H.264/AAC. Para buscar una posición en un flujo convertido usa **Comenzar en segundos**. Archivos dañados, DRM y formatos sin decodificador pueden no reproducirse.

## Móviles

Android incluye un APK firmado para Android 8 o posterior. Configura la dirección HTTPS de tu servidor; el teléfono utiliza los servicios de ese servidor. La UI y el puente móvil se han probado en Chromium táctil y WebKit, junto con las pruebas unitarias y lint de Android. No se han probado dispositivos físicos.

iOS incluye el proyecto fuente para Xcode, no una IPA compilada o firmada. Hace falta un Mac y, para distribución, la firma de Apple. Consulta `mobile/README.md`. GitHub Actions está bloqueado para esta cuenta y no se ejecutaron sus compilaciones nativas.

## Fuentes y compilación

`Teldrive-v2-fuentes.zip` incluye ambos proyectos y `source-revisions.json`. El código Teldrive está en `teldrive/` y el rclone modificado en `rclone/`. No incluye credenciales, almacenes de firma, cachés ni bases personales. Las revisiones y hashes de los paquetes se registran en el manifiesto de publicación.

Compila la UI antes del servidor: `scripts/build-cli.ps1`. Requiere Go 1.27 y Bun. Los contratos se regeneran desde TypeSpec y SQL según `AGENTS.md`. El paquete Windows se prepara con `desktop/build-payload.py` y `dotnet publish`; sus componentes y licencias se enumeran en `components.json`. Android usa Gradle 8.11.1, JDK 17/21 y SDK 36. Para problemas de sockets temporales de Java en Windows puede usarse `-Djdk.net.unixdomain.tmpdir=C:/ruta-corta` con una carpeta existente.

Se conservan los avisos y licencias de los proyectos y sus dependencias. La rama upstream v2 obtenida no contiene un archivo LICENSE raíz; no se le atribuye automáticamente la licencia de v1. rclone conserva su licencia MIT en `COPYING`. Los componentes externos de Windows conservan sus licencias propias, incluidas GPL/LGPL para FFmpeg/mpv.
