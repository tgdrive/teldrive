# Teldrive móvil v2

Clientes para Android y iPhone/iPad que abren la misma UI del servidor Teldrive v2. Incluyen conexión configurable, sesión persistente, selector de archivos del sistema, multimedia y navegación a los temas visuales. Los archivos, PostgreSQL, FFmpeg y rclone se ejecutan en el servidor; no se empaquetan esos servicios en el teléfono.

## Conexión

Introduce la raíz de tu servidor (`https://teldrive.tudominio.com`). La app inicia sesión mediante el flujo de Telegram del servidor. Usa HTTPS para acceso remoto. La opción HTTP local admite direcciones privadas y `.local`; no acepta HTTP público ni desactiva la validación de certificados.

`127.0.0.1` en el móvil apunta al móvil. Para conectar con Windows usa la dirección de su red o un dominio HTTPS y configura Teldrive para escuchar en esa interfaz. El modo de escritorio escucha en la interfaz local por defecto.

## Android (8.0 o posterior)

Con JDK 17 o 21, Android SDK 36 y Gradle 8.11.1, ejecuta en `android`:

```sh
gradle assembleRelease lintRelease testReleaseUnitTest
```

Para firmar tu APK crea un almacén privado y un `signing.properties` excluido de Git con `storeFile`, `storePassword`, `keyAlias` y `keyPassword`. Conserva la misma clave para instalar actualizaciones. Sin esos datos Gradle produce un APK sin firma.

Las descargas directas se guardan mediante el gestor de descargas de Android; conserva las cookies de sesión para archivos privados. Los enlaces públicos protegidos deben usar el enlace de descarga autorizado que proporciona la UI. Los dispositivos antiguos y algunos visores pueden tener límites de códecs: usa el modo compatible del servidor cuando esté disponible.

## iPhone/iPad (iOS 17 o posterior)

En macOS con Xcode y [XcodeGen](https://github.com/yonaskolb/XcodeGen):

```sh
cd ios
xcodegen generate
open Teldrive.xcodeproj
```

Selecciona tu equipo de Apple en Signing & Capabilities. Para comprobar el proyecto sin firma:

```sh
xcodebuild -project Teldrive.xcodeproj -scheme Teldrive -sdk iphonesimulator -configuration Release CODE_SIGNING_ALLOWED=NO build
```

Las descargas usan WKDownload y la hoja de compartir para guardarlas en Archivos. Xcode/macOS y las credenciales de firma de Apple son necesarios para producir una IPA instalable o publicar en TestFlight/App Store. El código de iOS no constituye una IPA firmada.
