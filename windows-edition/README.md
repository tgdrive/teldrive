# Teldrive para Windows — edición en español

Edición personalizada de [Teldrive](https://github.com/tgdrive/teldrive) con interfaz inspirada en Google Drive, configuración gráfica y un ejecutable Desktop separado de la CLI.

## Descargar y abrir

Los ejecutables se distribuyen en **Releases**, separados del código fuente. Descomprime `Teldrive-Windows.zip` y abre `Teldrive Desktop.exe`. El panel permite configurar el servidor, crear una base PostgreSQL local o conectar una existente e iniciar sesión con Telegram.

`teldrive.exe` es la edición de consola: `teldrive.exe run --config config.toml`. Evita el comando `upgrade` del proyecto original, porque puede reemplazar esta edición personalizada.

## Funciones

- Interfaz en español, seis temas, modo claro/oscuro/automático, listas y cuadrículas.
- Barra de almacenamiento, búsqueda, filtros y menús de archivos.
- Papelera recuperable y Spam manual; limpieza a los 30 días cuando está habilitada la tarea programada.
- Enlaces para compartir con contraseña, caducidad y revocación.
- Audio y vídeo originales o conversión compatible mediante FFmpeg.
- Configuración gráfica de 84 opciones del servidor.
- Integración con el fork de rclone para Teldrive: comprobación, copias, descargas y montaje mediante WinFsp.
- PostgreSQL estándar con búsqueda alternativa cuando PGroonga no está disponible.

Desktop abre la interfaz local mediante un perfil propio de Microsoft Edge o el navegador predeterminado. Los enlaces compartidos necesitan un servidor accesible desde el dispositivo receptor. El límite de almacenamiento configurable es una referencia personal, no una cuota de Telegram.

Consulta [las instrucciones de Windows](docs/Windows.md) para conocer los componentes incluidos, el cierre del programa y las limitaciones de reproducción.

## Código y compilación

- `teldrive/`: backend Go y lanzador Windows en `tools/desktop/`.
- `teldrive-ui/`: interfaz TypeScript/React.
- `RECOMPILAR.ps1`: compilación en Windows x64 con Go y Bun.

Consulta [FUENTES.md](FUENTES.md). Los componentes binarios de `tools/desktop/bundle` se entregan aparte del historial Git; la compilación de Desktop requiere ese directorio del paquete de fuentes. Las configuraciones personales, sesiones y bases de datos no forman parte del repositorio.

## Validación

Se comprobaron tipos y pruebas del frontend, paquetes del backend, permisos del panel, PostgreSQL local, migraciones sin PGroonga y conversiones reales de audio/vídeo. No se verificaron transferencias con una cuenta Telegram real. Los ejecutables personalizados no tienen firma digital.

## Proyectos originales y licencias

Esta edición no es una publicación oficial de tgdrive. Se basa en Teldrive 1.8.3 (`d400a2df41db17ba220cd06973fc8df5c6f2854c`) y Teldrive UI (`54ca894d82429718c86afb4bec1ac17f2332c650`).

El backend y el lanzador conservan MIT; la UI conserva GPLv3. Las licencias originales están en sus respectivos directorios. Los componentes externos tienen sus propias licencias y atribuciones, detalladas en las instrucciones de Windows.
