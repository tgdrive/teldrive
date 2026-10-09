# Teldrive Desktop v2

Aplicación para Windows 10/11 de 64 bits, construida con .NET 10, WPF y WebView2. El paquete incluye el servidor Teldrive, el rclone compatible con la API v2, PostgreSQL, FFmpeg y los instaladores oficiales de WinFsp, WebView2 y Visual C++. El primer inicio presenta la configuración del servidor y abre la UI en la propia ventana.

## Integración de rclone

En **Configuración → Rclone**, crea una clave de API de Teldrive y guarda la conexión en este equipo. El explorador integrado permite:

- Navegar por Teldrive y una carpeta local elegida mediante el selector de Windows.
- Subir archivos y carpetas, descargar, renombrar, crear carpetas y eliminar elementos mediante el cliente compatible.
- Revisar registros de transferencias en segundo plano, detenerlas y reintentarlas. Las copias evitan sobrescribir archivos diferentes.
- Calcular tamaños, exportar la lista y crear enlaces públicos.
- Reproducir audio y vídeo con el mpv incluido, en una ventana del programa. rclone sirve el archivo por una conexión local autenticada mientras dura la reproducción, con acceso de lectura y soporte de búsqueda por rangos. La contraseña temporal se elimina al cerrar el reproductor.
- Cifrar `rclone.conf` con el formato propio de rclone. La contraseña se mantiene en memoria durante la sesión; vuelve a introducirla al abrir el programa. Bloquear la configuración detiene los procesos de rclone.
- Instalar el WinFsp incluido y montar una letra de unidad libre. El controlador requiere autorización de Windows; las transferencias no necesitan instalarlo.

El diseño toma como referencia el flujo de [Rclone Browser Portable](https://github.com/MinorMole/RcloneBrowser-Portable). Se utiliza el explorador propio de Teldrive; no se distribuye el programa de ese proyecto. La integración administra la conexión de Teldrive de este equipo. Otras conexiones y las actualizaciones automáticas no están implementadas en este explorador. mpv procede de la compilación CI del propio proyecto y su origen, hash y licencias se incluyen en el paquete.

## Modo portátil

Ejecuta `"Teldrive Desktop v2.exe" --portable` o crea un archivo vacío `Teldrive.portable` junto al ejecutable. La configuración, PostgreSQL, cachés y componentes se guardan en `TeldriveData` al lado del programa. Puedes mover juntos el ejecutable y esa carpeta con el programa cerrado. Usa una ubicación NTFS con permiso de escritura; el programa aplica permisos privados a los datos. El controlador WinFsp y los componentes de Microsoft se instalan en Windows y permanecen en ese equipo.

Por defecto, los datos se guardan en `%LOCALAPPDATA%\TeldriveV2`. No se reutiliza automáticamente la base de datos de la edición anterior. La configuración del servidor contiene credenciales de PostgreSQL y claves de seguridad: conserva la carpeta de datos en una ubicación privada. El cifrado de rclone protege su archivo de configuración, no toda la carpeta de datos.

## Validación

`--smoke-test --data-root RUTA_NUEVA` comprueba extracción e inicialización de WebView2. `--smoke-rclone --data-root RUTA_NUEVA` ejecuta el cliente incluido sobre una conexión local aislada y verifica cifrado, contraseña incorrecta, desbloqueo, navegación, carpetas, subida, descarga, renombrado, tamaño, reintento, eliminación y bloqueo. Estas pruebas no instalan controladores ni acceden a Telegram. La prueba de integración del backend verifica por separado rclone frente a la API v2 y PostgreSQL.

`--smoke-server --data-root RUTA_NUEVA` prepara PostgreSQL, inicia Teldrive, verifica la pantalla de acceso en WebView2 y apaga los servicios. Para compilar, genera primero la UI y el servidor mediante `scripts/build-cli.ps1`; después prepara `Payload.zip` con `build-payload.py` y publica `dotnet publish desktop/Teldrive.Desktop.csproj -c Release -r win-x64`. Las compilaciones de la UI y del servidor se ejecutan consecutivamente para que el binario incluya el índice completo de la web.
