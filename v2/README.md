# Teldrive Windows Desktop 2.0.0.3

Actualización del Windows portátil de Teldrive v2. **Opciones adicionales de rclone** reúne tareas guardadas, cola, programador y preferencias de bandeja. El icono del avión y las nubes tiene fondo transparente.

Descargas: [GitLab](https://gitlab.com/ridrogo/teldrive/-/releases/v2.0.0-drive-es.3) · [GitHub](https://github.com/webr0m/teldrive/releases/tag/v2.0.0-drive-es.3).

Las tareas permiten copiar, mover, sincronizar, verificar y simular, con filtros, límites de velocidad, reintentos y comparación. La cola conserva pendientes e historial, controla hasta tres operaciones simultáneas y permite pausa, prioridades, cancelación y reintentos. El programador admite una ejecución, frecuencia diaria/semanal o intervalos y evita superponer una misma tarea. Las transferencias del explorador utilizan la cola.

En Preferencias puedes minimizar/cerrar hacia la bandeja, comenzar en ella e iniciar con Windows. Los horarios requieren el programa abierto, incluso en la bandeja. El inicio automático es opcional y se activa al guardar esa preferencia. La configuración cifrada de rclone requiere desbloqueo cada sesión.

Esta release actualiza Windows x64 y sus fuentes. Los CLI multiplataforma, Android e iOS siguen en [v2.0.0-drive-es.2](https://gitlab.com/ridrogo/teldrive/-/releases/v2.0.0-drive-es.2). El paquete completo integra el servidor, rclone compatible, PostgreSQL, multimedia y los instaladores de WinFsp y Microsoft.

## Fuentes y verificación

`teldrive/` contiene backend, UI, Desktop .NET/WPF y los proyectos móviles; `rclone/` conserva el cliente compatible. `source-revisions.json` registra los commits originales y los archivos se verifican por blob y permisos Unix.

El ejecutable pasó copias/descargas reales, filtros, simulación, pausa, prioridad, concurrencia, cifrado/bloqueo, cancelación, reintento, sincronización con autorización, persistencia de tareas/horarios, controles WPF y ciclo de bandeja. También se comprobó el servidor con PostgreSQL/migraciones, la pantalla de acceso, inicio oculto y actualización desde .2 conservando la configuración. Pasaron las cuatro pruebas del explorador en Chromium, la comprobación de tipos y la compilación de la UI. Windows detecta transparencia en el icono del ejecutable. No se utilizó una cuenta real de Telegram ni se modificó el inicio de Windows durante las pruebas.

Consulta `teldrive/desktop/README.md`. La release contiene `VALIDACION.json`, `BUILD-MANIFEST.json` y `SHA256SUMS.txt`.
