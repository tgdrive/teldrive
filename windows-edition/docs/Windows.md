# Teldrive para Windows — edición personalizada

## Abrir como programa

Haz doble clic en **Teldrive Desktop.exe**. Es un ejecutable distinto de `teldrive.exe` y abre una ventana de la interfaz mediante Microsoft Edge; si Edge no está disponible, usa tu navegador predeterminado. No abre una consola.

La apertura utiliza un perfil propio de Edge, conservado en `%LOCALAPPDATA%\Teldrive Desktop\WebProfile`, para evitar que una instancia de Edge en segundo plano absorba la ventana. Si la ventana no puede iniciarse, muestra un mensaje de error. `Teldrive Desktop-corregido.exe` es una copia de esta misma versión corregida con un nombre alternativo.

1. En la primera ejecución, pulsa **Crear / usar base de datos local incluida**. También puedes conectar una base PostgreSQL existente o importar tu configuración TOML.
2. Pulsa **Iniciar Teldrive**, después **Abrir mi unidad**, e inicia sesión con Telegram.
3. En **Configuración → rclone**, pulsa **Usar mi sesión actual**. Puedes comprobar la conexión, simular copias, copiar, descargar y montar una unidad con botones. El progreso aparece en la misma página.
4. Para montar una letra de unidad, instala WinFsp con el botón de la interfaz si aún no está instalado. Windows mostrará su instalador oficial y pedirá permisos de administrador. No hace falta WinFsp para copiar o descargar.

El ejecutable incluye Teldrive, tgdrive/rclone 1.73.1, PostgreSQL 17.11, FFmpeg y el instalador oficial WinFsp 2.1.25156. Extrae estos componentes en `%LOCALAPPDATA%\Teldrive Desktop`, donde conserva la configuración y la base local. No modifica tus otras instalaciones o bases existentes. El primer arranque necesita espacio para descomprimir los componentes.

Si el equipo no tiene el runtime de Microsoft Visual C++ que utiliza PostgreSQL, la interfaz ofrecerá descargar e iniciar su instalador oficial desde Microsoft. No se incluye ese runtime propietario en el paquete. El equipo de prueba ya disponía de él.

El servidor del programa y su panel de administración escuchan solamente en el equipo local. El puerto de la interfaz se conserva entre arranques. Cerrar la ventana deja el programa funcionando; usa **Cerrar programa** para terminar Teldrive, rclone y PostgreSQL. Abrir el ejecutable otra vez recupera la interfaz de la instancia existente. Si eliges la base local en un equipo nuevo, empieza vacía: para conservar tu unidad anterior, importa su conexión y configuración.

## Cambios de la interfaz

- Navegación inspirada en Drive, menús de archivos, búsquedas, listas y cuadrículas, almacenamiento con icono de nube y barra debajo de su menú.
- Español, seis temas configurables, modos claro/oscuro/automático y densidad ajustable. Información aparece al final de Configuración.
- Compartir mediante enlaces, contraseña, caducidad y revocación. Los enlaces de un servidor local necesitan un servidor accesible desde el dispositivo receptor.
- Papelera recuperable y sección Spam manual. Filtros de tipo, fecha de modificación y vistas. La eliminación a los 30 días requiere la limpieza programada activa. La eliminación física de Telegram la termina esa limpieza.
- Audio y vídeo originales o en modo compatible mediante FFmpeg. El modo compatible convierte a MP3 o H.264/AAC, con selección del segundo inicial; no ofrece búsqueda instantánea por toda la duración de una conversión en curso. La compatibilidad depende de los códecs y del estado del archivo.
- 84 opciones del servidor disponibles en el formulario de Desktop y para exportar TOML desde la UI web. El panel web Servidor y CLI exporta configuraciones/comandos; el panel Desktop guarda y aplica la configuración al arrancar el servidor.
- PostgreSQL estándar funciona sin PGroonga usando búsqueda por texto y expresiones regulares. Si la base ya dispone de PGroonga, se conserva su búsqueda. La búsqueda alternativa no interpreta el lenguaje avanzado de consultas de PGroonga.

El límite de almacenamiento es una referencia personal configurable; no representa una cuota impuesta por Telegram.

## Ejecutable de consola separado

`teldrive.exe` conserva `run`, `check`, `version` y la ayuda de CLI. `rclone.exe` es el fork compatible, disponible también por separado. Para usar reproducción compatible en la edición CLI, establece `server.media.ffmpeg-path` a la ruta de FFmpeg; Desktop lo configura automáticamente para sus componentes incluidos.

No uses `upgrade` de la CLI para actualizar esta edición: puede reemplazarla con el binario oficial y quitar los cambios personalizados.

## Verificación y fuentes

Se verificaron la compilación de Windows, TypeScript, pruebas del frontend y backend, creación/reapertura de PostgreSQL local, migraciones sin PGroonga, control de acceso del panel y conversión/decodificación real de FLAC y MKV mediante FFmpeg. No se comprobó una transferencia o reproducción con una cuenta Telegram real. El instalador de WinFsp no se ejecutó durante estas pruebas; el equipo ya tenía WinFsp instalado.

WinFsp - Windows File System Proxy, Copyright (C) Bill Zissimopoulos.
Proyecto y licencia: https://github.com/winfsp/winfsp
Instalador sin modificar, firma Authenticode válida de NAVIMATICS LLC. SHA256 publicado: `073A70E00F77423E34BED98B86E600DEF93393BA5822204FAC57A29324DB9F7A`.

Componentes y fuentes originales:

- Teldrive 1.8.3, base d400a2df41db17ba220cd06973fc8df5c6f2854c: https://github.com/tgdrive/teldrive — MIT.
- Teldrive UI, base 54ca894d82429718c86afb4bec1ac17f2332c650: https://github.com/tgdrive/teldrive-ui — GPLv3.
- rclone personalizado 1.73.1: https://github.com/tgdrive/rclone/tree/v1.73.1 — MIT.
- PostgreSQL 17.11, binarios de EDB: https://www.enterprisedb.com/download-postgresql-binaries — PostgreSQL License y licencias de sus dependencias. Código: https://www.postgresql.org/ftp/source/v17.11/
- WinFsp 2.1: https://github.com/winfsp/winfsp/tree/v2.1 — GPLv3 con excepción FLOSS.
- FFmpeg Gyan 2026-02-09, revisión 9bfa1635ae, GPLv3: https://www.gyan.dev/ffmpeg/builds/ — código de FFmpeg: https://github.com/FFmpeg/FFmpeg/tree/9bfa1635ae

Las licencias incluidas están en `licencias`. El paquete `Teldrive-fuentes.zip` contiene las fuentes modificadas y las instrucciones para recompilar. `SHA256SUMS.txt` identifica los archivos entregados; los ejecutables personalizados no llevan firma digital de un editor.
