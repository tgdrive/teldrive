# Teldrive v2 en español

Esta edición incluye la UI en español con temas, almacenamiento, papelera, Spam, controles táctiles y multimedia compatible. El programa Windows usa .NET 10, WPF y WebView2, e integra PostgreSQL, rclone para la API v2, FFmpeg, mpv y los instaladores de WinFsp y Microsoft.

Descarga los paquetes en la [release v2.0.0-drive-es.1](https://github.com/webr0m/teldrive/releases/tag/v2.0.0-drive-es.1). Windows completo se ofrece como un único ZIP. También hay seis paquetes CLI con rclone para Windows/Linux/macOS x64 y ARM64, APK Android y código iOS.

## Fuentes

- `teldrive/`: backend, UI, aplicación Windows y proyectos Android/iOS.
- `rclone/`: cliente adaptado a la API v2 y sus pruebas de compatibilidad.
- `source-revisions.json`: revisiones exactas de los dos proyectos originales modificados.
- `teldrive/LEEME-V2.md`: configuración, compilación y limitaciones.

Los blobs y permisos de estos dos directorios coinciden con las revisiones registradas. No se incluyen claves de firma, configuraciones personales, bases de datos ni componentes binarios en Git. Para compilar en Linux/macOS utiliza el checkout Git, que conserva los permisos de los scripts.

## Validación

Se comprobaron todos los paquetes Go e integración con PostgreSQL, aislamiento de Spam, reproducción pública protegida y conversión real con FFmpeg. La UI pasó 64 pruebas de escritorio y 24 de controles móviles; Spam y reproducción se comprobaron en Chromium, Chromium táctil y WebKit. Android compila, pasa lint y sus pruebas unitarias, y su APK tiene firma verificada.

El ejecutable Windows distribuido pasó las pruebas de arranque con PostgreSQL, migraciones, UI de acceso, rechazo de una dirección no local, parada de servicios, transferencias, configuración cifrada, reproducción mpv y montaje real con WinFsp. Linux x64 arrancó con PostgreSQL y la UI final. Consulta el manifiesto y el informe de validación en la release.

iOS se entrega como proyecto fuente, sin IPA compilada ni firmada. Hace falta un Mac/Xcode y firma Apple para distribuirlo. No hay pruebas en teléfonos físicos, hardware macOS/ARM64 ni una cuenta real de Telegram. Los rclone de macOS permiten transferencias; el montaje de macOS ARM64 no está incluido en estas compilaciones sin CGO. El workflow nativo se conserva para cuando haya un ejecutor disponible; no aporta resultados de compilación a esta release.
