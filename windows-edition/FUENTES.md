# Recompilar esta edición

Los directorios `teldrive` y `teldrive-ui` contienen las fuentes modificadas. El lanzador separado de Windows está en `teldrive/tools/desktop`; su código es MIT. La interfaz de Teldrive conserva su licencia GPLv3.

En Windows x64, instala Go y Bun para desarrollar y ejecuta `./RECOMPILAR.ps1`. El script instala las dependencias del frontend, comprueba tipos y pruebas, genera la API desde la especificación local, compila el servidor con la UI personalizada y compila `Teldrive Desktop.exe` con subsistema gráfico Windows. No descarga ni reemplaza la interfaz por la oficial.

El directorio `tools/desktop/bundle` del archivo de fuentes conserva los componentes de terceros usados en el programa: rclone/FFmpeg/WinFsp comprimidos con gzip, el ZIP reducido de PostgreSQL (`bin`, `lib`, `share` y licencias) y los textos de licencia. El servidor y su esquema se reconstruyen desde estas fuentes. No contiene sesiones Telegram, contraseñas del usuario o bases de datos.

Las bases para reconstruir o revisar cambios son Teldrive 1.8.3 (`d400a2df41db17ba220cd06973fc8df5c6f2854c`) y UI (`54ca894d82429718c86afb4bec1ac17f2332c650`). La especificación OpenAPI está fijada en `teldrive/spec/UPSTREAM_COMMIT`.

Las pruebas opcionales usan exclusivamente bases aisladas:

- `TELDRIVE_DESKTOP_DB_TEST=1`: crea PostgreSQL en un directorio temporal y prueba creación/reapertura. No modifica la base Desktop real.
- `TELDRIVE_MIGRATION_TEST_DSN`: debe apuntar a una base cuyo nombre termine en `_migration_test`; prueba las migraciones en PostgreSQL estándar.
- `TELDRIVE_LIFECYCLE_TEST_DSN`: base de prueba de papelera/spam, con las restricciones del propio test. No apuntar a una base de usuario.

`go test ./cmd ./internal/... ./pkg/... ./tools/desktop` comprueba los paquetes del programa. Los tests antiguos de `tests/integration` y `tests/performance` del proyecto original tienen su conexión sin configurar y necesitan adaptar ese entorno antes de ejecutar `go test ./...`.

Las fuentes y licencias de los componentes externos se referencian en `dist-custom/windows-amd64/LEEME.md`. El runtime de Microsoft no se redistribuye dentro del programa: en equipos que no lo tengan, el panel ofrece descargar e iniciar su instalador directamente desde Microsoft, con verificación del hash de la versión comprobada. Windows muestra los permisos y las condiciones del instalador.
