# PyCast (Go)

Transmisor de audio nativo para **SHOUTcast v2** con interfaz gráfica en Windows, reescrito 100% en **Golang**.

Captura el audio del sistema (loopback WASAPI) o un micrófono, lo codifica a MP3 en tiempo real y lo envía a un servidor SHOUTcast DNAS v2 usando el **protocolo nativo SHOUTcast 2 (Ultravox 2.1)**.

Compila en un único binario ejecutable (`PyCast.exe`), sin requerir Python, DLLs externas ni herramientas CGO/GCC instaladas.

## ¿Por qué el protocolo nativo?

Los encoders clásicos que usan el protocolo SHOUTcast v1 (contraseña por el puerto +1) no pueden elegir el Stream ID en un DNAS 2.x: el audio termina montado en un stream nuevo (sid 1) y tu enlace de escucha queda "offline" aunque el encoder diga que está transmitiendo. PyCast implementa el handshake Ultravox 2.1 completo (autenticación cifrada con XTEA, negociación de stream y envío de datos encapsulado), por lo que el audio llega al Stream ID correcto en servidores multi-stream.

## Características

- **100% Go nativo (sin CGO)**: compilación directa en Windows sin necesidad de MinGW ni GCC.
- **Protocolo SHOUTcast 2 nativo (Ultravox 2.1)** con selección de Stream ID y cifrado XTEA.
- **Captura WASAPI en tiempo real**: loopback del sistema (altavoces/auriculares) o micrófonos/entradas con detección del dispositivo predeterminado.
- **Inyección automática de silencio**: mantiene la conexión viva incluso cuando no hay música reproduciéndose.
- **Codificador MP3 en Go puro**: bitrates seleccionables a 128 / 192 / 256 / 320 kbps con remuestreo automático a 48000 Hz / 44100 Hz.
- **VU meter estéreo en tiempo real** (canales L y R).
- **Gestión de presets**: guarda, carga y elimina configuraciones de servidores en `presets.json`.
- **Estadísticas al aire**: cronómetro en vivo y contador de megabytes transmitidos.
- **Manejo de errores claro**: detección de "Stream ocupado (AutoDJ)", credenciales incorrectas, SID inválido, etc.
- **Interfaz multiidioma**: inglés (por defecto) y español, con selector de idioma al iniciar y preferencia guardada en `config.json`.

## Requisitos

- Windows 10 u 11 (x64)
- Go 1.20 o superior (solo necesario si compilas desde el código fuente)

## Compilación y Ejecución

### Ejecutar directamente:
```powershell
go run .
```

### Compilar el binario ejecutable (.exe):
```powershell
go build -o PyCast.exe .
```

O para compilar como aplicación GUI sin ventana de terminal de fondo:
```powershell
go build -ldflags "-H windowsgui" -o PyCast.exe .
```

> **Importante (manifiesto):** la GUI usa la librería Walk, que requiere Common Controls 6. El manifiesto [`PyCast.manifest`](PyCast.manifest) ya está embebido en `rsrc.syso`, por lo que el build funciona directamente. Si modificas el manifiesto, regenera el recurso antes de compilar:
> ```powershell
> go run github.com/akavel/rsrc@v0.10.2 -manifest PyCast.manifest -o rsrc.syso
> ```
> Sin el manifiesto, la ventana no se crea y el programa se cierra al instante (`TTM_ADDTOOL failed`).

## Uso

1. Elige el **Dispositivo de Audio** en el menú desplegable.
2. Ingresa **Hostname or IP**, **Port**, **Stream ID (SID)**, **DJ Username** y **Password**.
3. Selecciona el **Bitrate** deseado (ej. 256 kbps).
4. Pulsa **Iniciar Broadcast**.

### Dispositivos: `[Loopback]` vs Entradas Directas

- **Con `[Loopback]`**: Copia digital directa de lo que suena por esa salida (altavoces o audífonos). Es la opción estándar para transmitir música o el audio completo de tu PC.
- **Sin `[Loopback]`**: Dispositivos de entrada físicos o virtuales (micrófonos, entradas de línea, cables virtuales VB-Audio).

## Idiomas

La interfaz está disponible en **inglés** (idioma por defecto) y **español**. Al iniciar la aplicación se abre una **ventana independiente de selección de idioma** (cuyo texto está siempre en inglés); lo que elijas define el idioma de toda la app y se guarda en `config.json`, así que se recuerda en las siguientes ejecuciones.

Las traducciones viven en [pkg/i18n/](pkg/i18n/) (`en.json`, `es.json`) y se **embeben dentro del `.exe`** con `//go:embed`, por lo que el binario sigue siendo autónomo. El inglés es el idioma base: cualquier clave que falte en español cae automáticamente al inglés.

## Estructura del Proyecto en Go

- [main.go](main.go) / [cmd/pycast/main.go](cmd/pycast/main.go) — Puntos de entrada (wrappers de `app.Run()`)
- [pkg/app/](pkg/app/) — Interfaz gráfica Win32 nativa (Walk), estado de conexión, cambio de idioma y bucle de streaming
- [pkg/i18n/](pkg/i18n/) — Motor de traducciones (inglés/español) embebidas en el binario
- [pkg/config/](pkg/config/) — Preferencias del usuario persistidas en `config.json` (p. ej. el idioma)
- [pkg/uvox/](pkg/uvox/) — Implementación completa del protocolo Ultravox 2.1 (framing, cifrado XTEA, handshake y envío MP3)
- [pkg/mp3/](pkg/mp3/) — Codificador MP3 en Go puro para streaming en tiempo real (capas MDCT, subband, Huffman, reservoir)
- [pkg/audio/](pkg/audio/) — Captura de audio WASAPI (loopback y micrófonos), remuestreo y cálculo de picos VU
- [pkg/presets/](pkg/presets/) — Gestor persistente de servidores en `presets.json`
- [cmd/test_encode/](cmd/test_encode/) — Herramienta de prueba de captura y codificación MP3
- [cmd/test_sc/](cmd/test_sc/) — Herramienta de prueba de dispositivos de sonido

## Licencia

GPL-3.0 — ver [LICENSE](LICENSE).
