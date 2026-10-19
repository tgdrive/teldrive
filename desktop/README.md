# Teldrive Desktop v2

Aplicación para Windows 10/11 de 64 bits, construida con .NET 10, WPF y WebView2. El paquete incluye el servidor Teldrive, el rclone compatible con la API v2, PostgreSQL, FFmpeg/FFprobe y los instaladores oficiales de WinFsp, WebView2 y Visual C++. El primer inicio presenta la configuración del servidor y abre la UI en la propia ventana.

## Reproducción y diagnóstico (2.0.0.10)

Pantalla completa está disponible en el vídeo de la ventana web y en el reproductor
nativo. En el nativo usa el botón, F/F11 o doble clic; Esc restaura la ventana y su
posición. Los menús de bandeja ejecutan sus acciones después de cerrar el menú
nativo, para evitar errores de reentrada de WPF/WebView2.

Activa **Desarrollador · estadísticas** durante la reproducción de vídeo o audio.
Desktop nativo consulta mpv por IPC: contenedor, códecs, resolución, FPS, tasa de
bits, audio, aceleración, fotogramas descartados, sincronización A/V, búfer,
velocidad, pausa y posición. En la ventana web se muestran los datos que el navegador
ofrece y las pistas detectadas por FFprobe. Un campo no disponible no se estima.

Las estadísticas de Telegram identifican cada cuenta que realmente recibe datos
con nombre, @alias e ID, bytes, velocidad móvil de dos segundos, solicitudes activas,
latencia, errores y último canal/mensaje/offset. Suman las lecturas del archivo en
el servidor (incluidas búsquedas y reproducciones simultáneas); no representan una
medición exclusiva de una ventana. Una velocidad cero puede significar que ya hay
contenido en búfer. Las identidades solo están disponibles para el propietario,
no mediante enlaces públicos. No se muestran claves, tokens ni URL de reproducción.
El origen nativo debe ser una conexión Teldrive; otros tipos de rclone conservan
los datos de mpv y muestran que la telemetría de Telegram no está disponible.

Se integra el original `tgdrive/teldrive` hasta `67bf180`: provisión independiente
de bots, selección y control de carga de cuentas, favoritos/recientes, preferencias
por cuenta, tamaños de carpetas, orden natural y mejoras de lectores/búsqueda.
Las migraciones propias 8–10 conservan su numeración y las nuevas ocupan 11–16.
Una reconciliación aditiva reconoce las tablas de las migraciones 8–12 del
original y conserva los datos existentes al cambiar a esta edición.

`--smoke-player --data-root RUTA_NUEVA` comprueba reproducción local real con mpv,
IPC, pausa, pantalla completa/restauración, estadísticas y callbacks de bandeja.
No usa credenciales ni tráfico de Telegram.

## Crear bots desde Configuración (2.0.0.8)

Desde la versión **2.0.0.9**, la lista presenta el **nombre**, el **@alias** y el
**ID** en líneas separadas y se ordena por nombre. El selector de gestor y la
confirmación al eliminar incluyen los tres datos. Si faltan datos antiguos, el
servidor los recupera desde Telegram; los bots pendientes conservan su estado y
sus permisos. Si Telegram no responde o el token es inválido, se mantiene el ID y
se indica que el nombre o alias está pendiente.

En **Configuración → Bots**, importa una vez el token de un bot gestor y activa
**Bot Management Mode** en sus ajustes de BotFather. Selecciónalo y pulsa
**Verificar gestor**. Después introduce el nombre y el usuario del nuevo bot,
comprueba la disponibilidad y pulsa **Crear y añadir bot**.

La creación usa tu sesión de Telegram. El servidor obtiene el token con el gestor,
lo guarda cifrado y programa la verificación e incorporación a los canales existentes.
Si se interrumpe, repite con el mismo usuario y gestor para recuperar tu bot.
La importación de tokens existentes sigue disponible. Requiere el servidor en modo
Telegram remoto; el modo de pruebas local no crea bots reales.

## Dominio para enlaces compartidos

En **Configuración → Configuración avanzada**, agrega `public-url = "https://drive.tudominio.com"`
en la sección `[http]` existente y guarda. El servidor generará los enlaces públicos
con ese dominio aunque estés usando la ventana local de Desktop. Configura el túnel
o proxy por separado; dejar `public-url` vacío conserva los enlaces con la dirección
de la interfaz. Los tokens de enlaces anteriores siguen funcionando con el nuevo dominio.

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

## Tareas, cola y programador (Windows 2.0.0.3)

Abre **Opciones adicionales de rclone** en la barra superior de Desktop o desde el explorador de rclone. Sus cuatro pestañas están en español:

- **Tareas:** crear, editar, duplicar, importar y exportar definiciones; ejecutar o simular sin modificar archivos. Admiten subida/descarga, copia, movimiento, sincronización y verificación. Puedes ajustar transferencias, verificadores, reintentos, límites de velocidad, tiempos de espera, comparación, profundidad, tamaños, antigüedad y patrones de inclusión/exclusión. Las opciones que eliminan archivos requieren autorización en la propia tarea; las tareas importadas pierden esa autorización para permitir revisar sus rutas.
- **Cola:** pausa/reanudación, una a tres tareas simultáneas, cambio de prioridad, cancelación, reintentos, registros exportables y limpieza del historial. Las transferencias iniciadas desde el explorador también utilizan esta cola. Pausar impide iniciar tareas nuevas y permite terminar las que ya están en curso. Un montaje ocupa una de las tres plazas de rclone. Se conservan hasta 1000 pendientes y 200 registros finalizados.
- **Programador:** una vez, diariamente, semanalmente o por intervalo de minutos; primera fecha y hora locales, activar/desactivar, editar y eliminar horarios. Una tarea pendiente o en ejecución no se vuelve a añadir por un segundo horario. Puedes ejecutar una vez los horarios perdidos al reabrir o dejarlos omitidos. El programador necesita Teldrive abierto y puede funcionar en la bandeja; no despierta el equipo apagado o suspendido.
- **Preferencias → Bandeja del sistema:** mostrar siempre el icono, minimizar o cerrar hacia la bandeja, abrir directamente en ella, iniciar con Windows en la bandeja y notificar transferencias terminadas. Haz clic en el icono para recuperar la ventana; **Salir y detener servicios** apaga los procesos del programa. Abrir de nuevo el ejecutable recupera la instancia existente. La primera configuración y los errores de inicio muestran la ventana para poder resolverlos.

El inicio con Windows es opcional y se registra para el usuario actual, sin privilegios de administrador. Se desactiva en las mismas preferencias. Si mueves el programa portátil y utilizas esta opción, vuelve a guardar las preferencias para actualizar la ubicación del ejecutable. Las tareas utilizan rutas locales absolutas: revisa las rutas si cambia la letra o ubicación de tus unidades.

`rclone-automation.json` y `desktop-preferences.json` quedan en la carpeta de datos privada junto a la configuración. Los horarios y tareas no guardan la contraseña de rclone; una configuración cifrada necesita desbloquearse cada sesión antes de continuar la cola. Las tareas interrumpidas al cerrar se conservan y requieren reintento; las pendientes permanecen en la cola. El icono de la aplicación y de la bandeja utiliza el avión proporcionado por el usuario, con fondo PNG transparente.

## Modo portátil

Ejecuta `"Teldrive Desktop v2.exe" --portable` o crea un archivo vacío `Teldrive.portable` junto al ejecutable. La configuración, PostgreSQL, cachés y componentes se guardan en `TeldriveData` al lado del programa. Puedes mover juntos el ejecutable y esa carpeta con el programa cerrado. Usa una ubicación NTFS con permiso de escritura; el programa aplica permisos privados a los datos. El controlador WinFsp y los componentes de Microsoft se instalan en Windows y permanecen en ese equipo.

Por defecto, los datos se guardan en `%LOCALAPPDATA%\TeldriveV2`. No se reutiliza automáticamente la base de datos de la edición anterior. La configuración del servidor contiene credenciales de PostgreSQL y claves de seguridad: conserva la carpeta de datos en una ubicación privada. El cifrado de rclone protege su archivo de configuración, no toda la carpeta de datos.

## Validación

`--smoke-automation --data-root RUTA_NUEVA` verifica copias/descargas reales con el rclone incluido sobre una conexión local aislada: filtros, simulación, pausa, prioridad, concurrencia, cifrado/bloqueo, reintento, autorización de sincronización, eliminación de sobrantes, horarios perdidos, ausencia de solapamientos, cancelación, persistencia al reiniciar, controles WPF e icono/minimización/cierre/restauración de bandeja. Guarda el resultado y cuatro capturas de las pestañas. No modifica el inicio automático de Windows ni utiliza Telegram.

`--smoke-startup-tray --start-in-tray --data-root RUTA_DE_PRUEBA_CONFIGURADA` comprueba el inicio real de los servicios en la bandeja y la recuperación de la ventana, sin registrar inicio automático en Windows. Usa exclusivamente una carpeta de pruebas preparada previamente con `--smoke-server`.

`--smoke-test --data-root RUTA_NUEVA` comprueba extracción e inicialización de WebView2. `--smoke-rclone --data-root RUTA_NUEVA` ejecuta el cliente incluido sobre una conexión local aislada y verifica cifrado, contraseña incorrecta, desbloqueo, navegación, carpetas, subida, descarga, renombrado, tamaño, reintento, eliminación y bloqueo. Estas pruebas no instalan controladores ni acceden a Telegram. La prueba de integración del backend verifica por separado rclone frente a la API v2 y PostgreSQL.

`--smoke-server --data-root RUTA_NUEVA` prepara PostgreSQL, inicia Teldrive, verifica la pantalla de acceso en WebView2 y apaga los servicios. Para compilar, genera primero la UI y el servidor mediante `scripts/build-cli.ps1`; después prepara `Payload.zip` con `build-payload.py` y publica `dotnet publish desktop/Teldrive.Desktop.csproj -c Release -r win-x64`. Las compilaciones de la UI y del servidor se ejecutan consecutivamente para que el binario incluya el índice completo de la web.

## Navegación, configuración personalizada y transmisión (2.0.0.5)

El botón superior **Teldrive** recupera el Drive desde Actividad, Configuración u Opciones adicionales de rclone. El WebView2 de ventana se oculta antes de mostrar los paneles WPF y evita el error gráfico `0x8876086C` de la superficie Direct3D9 del host de composición. Actividad conserva una cola de texto acotada.

**rclone.conf** importa una copia del archivo original, valida y edita sus conexiones sin borrar comentarios/opciones adicionales. Los valores privados están ocultos en la lista; el editor completo se abre expresamente. Guardar crea `rclone.conf.previous` y conserva el cifrado si estaba activado. Las transferencias y montajes deben estar detenidos al modificar el archivo. Las tareas pueden elegir el nombre de una conexión personalizada. En la UI web de un CLI, el editor importa y exporta el archivo local; no escribe en el servidor.

**Transmitir** usa un enlace temporal del vídeo. DLNA se controla desde la red del servidor; la búsqueda requiere administrador. Chromecast utiliza Chrome, que se abre desde Desktop con un reproductor dedicado. AirPlay requiere Safari en iPhone, iPad o Mac; el enlace y QR permiten reproducir el vídeo allí. El enlace de red local solo sirve vídeos/subtítulos autorizados y se cierra con el servidor; no expone configuración ni la UI principal. La red y el cortafuegos deben permitir que el televisor acceda al servidor.

FFprobe detecta las pistas de audio y subtítulos de MKV y otros contenedores. FFmpeg aplica el audio seleccionado, convierte subtítulos de texto a WebVTT e integra los de imagen en el vídeo compatible. La conversión incluye las pistas elegidas al transmitir. Los resultados dependen de los códecs disponibles y del dispositivo.

`--smoke-enhancements --data-root RUTA_NUEVA` comprueba el botón Teldrive, el registro de Actividad sin capa blanca, importación/edición/cifrado/respaldos de rclone.conf y una transferencia real hacia una conexión personalizada. Usa PostgreSQL y carpetas de prueba aisladas, sin Telegram ni televisores físicos.

## Corrección del doble clic (2.0.0.6)

El host de ventana WebView2 recibe el ratón directamente desde Windows. No usa
el reenvío de doble clic de WPF que duplicaba eventos en el control de composición.

`--smoke-input --data-root RUTA_DE_PRUEBA` verifica la apertura de carpetas en
lista y cuadrícula mediante el protocolo de entrada de Chromium. Usa una base
local aislada y respuestas de API simuladas; no modifica archivos del usuario.
Guarda el registro de eventos recibidos por la página.

## Montajes persistentes e idiomas (2.0.0.6)

En Opciones adicionales de rclone → Tareas elige «Montar unidad persistente», la conexión de rclone.conf, la carpeta remota (por ejemplo /) y una letra libre de D: a Z:. Puedes configurar caché VFS, límite, duración y solo lectura. Activa «Montar automáticamente al iniciar Teldrive Desktop», guarda la tarea y habilita «Iniciar automáticamente con Windows» en Preferencias. El montaje espera al servidor y a que rclone.conf esté desbloqueado. No guarda contraseñas. La cola puede continuar transfiriendo mientras la unidad está montada. «Cancelar / detener» desmonta la unidad; al cerrar Desktop también se desmonta y vuelve a montarse en el siguiente inicio. Una cola pausada espera a que se reanude. Las tareas importadas requieren activar nuevamente el montaje al iniciar.

Preferencias incluye español, inglés, portugués, francés, alemán, italiano, ruso, chino simplificado, japonés y árabe. La UI del CLI tiene el selector en Configuración → Apariencia. Las traducciones se incluyen sin conexión; son una primera traducción automática que puede revisarse en localization/catalog.json. Los nombres de archivos, las rutas y las credenciales se conservan. Árabe usa dirección de lectura de derecha a izquierda y conserva los campos técnicos de Desktop de izquierda a derecha. La ayuda técnica del terminal conserva su idioma original.

El tema claro utiliza un menú móvil opaco y las mismas variables de color en controles y menús. Los marcadores /my-drive, /recent y /settings/account redirigen a las pantallas actuales. El reproductor elige la pista predeterminada declarada por el contenedor; MKV y audio no compatible se convierten automáticamente cuando FFmpeg está disponible.

`--smoke-automation` añade una prueba real de montaje WinFsp, reinicio, espera del servidor/desbloqueo, supresión de duplicados, transferencia simultánea y desmontaje al cancelar. Usa exclusivamente una conexión alias y carpetas aisladas.
