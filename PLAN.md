# ai-compare — Plataforma local para comparar agentes de IA

## Problema

Quiero saber qué combinación de CLI, proveedor, modelo, esfuerzo y preset resuelve mejor una tarea **de mi trabajo real**, cuánto cuesta y cuánto tarda. Hoy no puedo comparar dos combinaciones sobre el mismo proyecto y el mismo prompt sin tocar mi entorno de desarrollo.

Los resultados de cada comparación son **desechables**: el código generado no se sube a GitLab ni a ningún remoto. Solo interesan las métricas (coste, tiempo, tokens), la calidad de la solución y el informe.

## Concepto

1. En el dashboard indico la **ruta absoluta** de un proyecto de mi equipo (por ejemplo `C:\Users\Jose\Desktop\projects\mi-app`).
2. Configuro dos lados, A y B. Cada lado tiene su CLI (`claude`, `codex` u `opencode`), proveedor, modelo, esfuerzo, preset y modo (interactivo o autónomo).
3. Escribo un prompt común.
4. Un script determinista, sin LLM, construye **una imagen Docker por lado**. Cada imagen contiene:
   - la copia del proyecto tal cual está;
   - el proyecto sin sus archivos de harness originales;
   - el preset elegido;
   - el CLI elegido.
5. Cuando las dos imágenes están listas, arranca ambos contenedores, abre el CLI de cada lado y le envía el prompt automáticamente.
6. El dashboard muestra las dos terminales **lado a lado**. En modo interactivo respondo desde ahí a las preguntas de cada agente.
7. Cuando ambos lados terminan, genero el informe: coste, tiempo, tokens, diff, tests y revisión de calidad.

Es una herramienta local de un solo usuario. Se clona desde GitHub y se levanta con Docker Compose. No hay cuentas, login ni despliegue alojado.

## Flujo de uso

1. **Proyecto.** Escribo o pego la ruta absoluta. La app valida que existe y muestra:
   - nombre y tamaño estimado de la copia;
   - número de archivos que se copiarán;
   - archivos de harness que se excluirán;
   - archivos sensibles detectados (`.env*`), excluidos por defecto.
2. **Perfil del proyecto.** La primera vez que uso una ruta, la app propone un perfil (runtime, comando de setup y comando de tests) a partir de lo que detecta, por ejemplo `package.json` y su lockfile. Lo confirmo o lo edito, y queda guardado para esa ruta.
3. **Prompt.** Lo escribo en un textarea.
4. **Lados A y B.** Elijo CLI, proveedor/modelo, esfuerzo, preset y modo. Las combinaciones incompatibles se deshabilitan con una explicación.
5. **Ejecutar.** Pulso **Ejecutar comparación**. Veo el progreso de la preparación: copia, setup, construcción y arranque.
6. **Durante la corrida.** Veo dos terminales lado a lado, el estado de cada lado y el coste y los tokens acumulados en vivo.
7. **Finalizar.** En modo autónomo, un lado termina cuando el CLI sale. En modo interactivo, termina cuando salgo del CLI o pulso **Finalizar lado**. También puedo cancelar. Un error o una cancelación en un lado no bloquea el informe.
8. **Informe.** Cuando ambos lados están en estado terminal (éxito, error o cancelado), se habilita **Generar informe**.
9. **Historial.** Todas las comparaciones quedan guardadas con su informe, diffs, logs y métricas.

## Páginas

| Ruta | Para qué sirve |
|---|---|
| `/` | Configurar y lanzar una comparación, y seguirla en vivo |
| `/harnesses` | Crear, editar y eliminar presets |
| `/pricing` | Precios de models.dev, solo lectura |
| `/history` | Lista de comparaciones agrupadas por fecha |
| `/history/:id` | Detalle de una comparación |
| `/settings` | Estado de las API keys y valores por defecto |

Una barra de navegación fija da acceso a todas. Si hay una comparación en curso, muestra un indicador que lleva a `/`.

### `/` — Nueva comparación

La página tiene dos estados: **configuración** y **ejecución**. Al lanzar, el formulario se pliega en una franja de resumen (proyecto, prompt truncado, A vs B) y la pantalla pasa a los dos paneles.

**Configuración:**

1. **Proyecto.**
   - Campo de ruta absoluta con un historial de rutas recientes.
   - Al validar, muestra nombre, tamaño, número de archivos, archivos de harness que se excluirán y `.env*` detectados.
   - Bloque plegable **Perfil del proyecto**: runtime, comando de setup, comando de tests, exclusiones adicionales y carpeta de tests ocultos. Se precarga con el perfil guardado para esa ruta o con la detección automática.
2. **Prompt.** Textarea grande con contador de caracteres.
3. **Lado A y lado B,** en dos columnas con los mismos controles:
   - **CLI:** `claude`, `codex` u `opencode`.
   - **Proveedor:** filtrado por los que admite el CLI y tienen API key configurada.
   - **Modelo:** filtrado por proveedor; la lista sale de models.dev (y, para modelos locales, del servidor local). Si el modelo no tiene precio en models.dev, aparece un aviso: el coste saldrá «no calculable».
   - **Esfuerzo:** deshabilitado si el CLI o el modelo no lo admiten.
   - **Preset:** los de `/harnesses` compatibles con el CLI, más **Sin harness** (opción fija que corre el CLI sin ningún archivo de harness). Un enlace abre el preset en otra pestaña.
   - **Modo:** interactivo o autónomo.
   - **Límites (opcionales):** timeout, tokens máximos y coste máximo. Cada uno tiene su propio interruptor y puede quedar **sin límite**. Se precargan desde `/settings`, donde por defecto están desactivados. El de coste no aparece con modelos locales.
   - Botones **Copiar A → B** e **Intercambiar A ↔ B**.
4. **Ejecutar comparación.** Deshabilitado hasta que todo sea válido; los errores se indican junto a cada campo.

**Ejecución:** dos paneles lado a lado, uno por lado. Cada panel tiene:

- **Cabecera:**
  - CLI · modelo · preset · modo;
  - estado: preparando, construyendo, arrancando, en curso, esperando entrada, finalizado, error, cancelado o límite alcanzado;
  - tiempo transcurrido, tokens y coste en vivo;
  - botones **Finalizar lado** y **Cancelar**.
- **Pestañas:**

  | Pestaña | Contenido |
  |---|---|
  | **Terminal** | TUI interactiva (xterm.js). En modo autónomo es de solo lectura. |
  | **Logs** | Log de preparación (copia, setup, build), de infraestructura y del proxy de inferencia (peticiones, 429, reintentos), con filtro por nivel. |
  | **Cambios** | Diff de solución contra la línea base, actualizable bajo demanda durante la corrida y definitivo al terminar. Incluye la lista de archivos modificados. |
  | **Métricas** | Tokens por categoría, coste acumulado (frente al límite, si lo hay), peticiones y tiempos: preparación, agente y espera humana. |
  | **Preview** | Fase 2: la aplicación corriendo en ese lado. |

- **Pie de página común**, cuando ambos lados están en estado terminal:
  - **Generar informe**, que lleva a `/history/:id` con el informe generándose;
  - **Nueva comparación**, que vuelve al formulario con la misma configuración precargada.

### `/harnesses` — Presets

**Lista:**

- Una card por preset con título, descripción, CLIs compatibles, número de archivos y fecha de última edición.
- Búsqueda por nombre.
- Botón **Nuevo preset**.
- **Sin harness** aparece como preset fijo del sistema: no se puede editar ni borrar.

**Crear** (`/harnesses/new`):

1. Nombre (obligatorio, del que sale el slug de la carpeta), descripción y CLIs compatibles.
2. Contenido, por cualquiera de estas vías y combinables:
   - **Importar desde un proyecto:** indico una ruta, la app detecta sus archivos de harness y muestra un checklist para elegir cuáles importar.
   - **Soltar carpetas o archivos:** `.claude/`, `.agents/`, `AGENTS.md`, `.mcp.json`, etc. Se colocan en `project/` respetando su ruta relativa.
   - **Soltar en `home/`:** una zona separada para la configuración de usuario, como `.codex/config.toml`.
3. Guardar crea la carpeta `harnesses/<slug>/` con `preset.md`, `project/` y `home/`.

**Detalle y edición** (`/harnesses/:slug`):

- **Cabecera:** título y descripción editables en línea, y CLIs compatibles.
- **Cards por categoría**, deducidas de las rutas: instrucciones (`AGENTS.md`, `CLAUDE.md`), skills, rules, agentes/subagentes, MCP y otros. Cada card lista sus archivos.
- **Árbol de archivos** con dos raíces, `project/` y `home/`.
- **Visor y editor:**
  - Markdown renderizado con un botón **Editar**, que abre un editor de texto (Markdown, JSON, TOML, YAML) con validación de sintaxis para JSON y TOML;
  - los secretos de MCP deben ir como referencias a variables de entorno; si el editor detecta un valor con aspecto de clave, avisa.
- **Añadir archivos** soltándolos sobre una carpeta del árbol; **renombrar, mover y eliminar** archivos desde el árbol.
- **Acciones del preset:**
  - **Duplicar**, para crear variantes;
  - **Renombrar**;
  - **Eliminar**, con confirmación. Las comparaciones antiguas no se ven afectadas porque guardan su propia copia del preset.
- **Uso:** número de comparaciones que lo usaron, con enlace a `/history` filtrado por ese preset.

### `/pricing` — Precios (solo lectura)

Muestra directamente los precios de **models.dev** (`https://models.dev/api.json`). No hay tabla de precios propia ni edición.

- **Filas:** una por modelo de los proveedores configurados (OpenAI en la fase 1; Anthropic y modelos locales después).
- **Columnas,** en USD por millón de tokens:
  - input;
  - input cacheado (lectura de caché);
  - escritura de caché;
  - output;
  - límite de contexto.

  «Output cacheado» no existe en ningún proveedor; la cuarta categoría real es la **escritura de caché**. Anthropic la cobra aparte, OpenAI no (se muestra «—»). Los tokens de razonamiento se facturan como output. Si models.dev publica tramos por contexto largo, se muestran como sub-filas.
- **Cabecera:** fecha de la última consulta a models.dev y botón **Actualizar**. Si la consulta falla, se muestra la copia en caché con su fecha y un aviso.
- **Filtros:** proveedor y buscador.
- **Modelos locales** (fase 2): aparecen como «local · sin coste», porque no están en models.dev ni se facturan.

Por qué models.dev: ni la API de OpenAI ni la de Anthropic devuelven precios (sus endpoints `/v1/models` solo listan modelos). models.dev es un catálogo público por proveedor y modelo, mantenido por el equipo de opencode, con `cost.input`, `cost.output`, `cost.cache_read`, `cost.cache_write` y límites de contexto.

El funcionamiento interno (caché, instantánea por comparación, modelos sin precio) está en [Precios](#precios).

### `/history` — Historial

- **Agrupado por fecha:** Hoy, Ayer, y luego una cabecera por día (por ejemplo, «martes 29 sep 2026»). Dentro de cada grupo, más recientes primero.
- **Una fila por comparación:**

  | Columna | Contenido |
  |---|---|
  | Hora | Hora de inicio |
  | Proyecto | Nombre de la carpeta, con la ruta completa en tooltip |
  | Prompt | Primeras palabras, truncado |
  | A | CLI · modelo · preset |
  | B | CLI · modelo · preset |
  | Estado | Estado de cada lado |
  | Coste | A / B, con el menor resaltado |
  | Duración | A / B, con la menor resaltada |
  | Tests | Resultado A / B |
  | Informe | Generado o pendiente |

- **Filtros:** texto (prompt o proyecto), rango de fechas, CLI, modelo, preset y estado. Los filtros se reflejan en la URL para poder enlazarlos.
- **Paginación o scroll infinito** por grupos de fecha.
- Clic en una fila → `/history/:id`.

### `/history/:id` — Detalle de comparación

1. **Cabecera:** fecha y hora, proyecto (ruta), duración total y estado. Acciones:
   - **Repetir comparación:** abre `/` con todo precargado;
   - **Generar/Regenerar informe**;
   - **Eliminar comparación**, con confirmación;
   - **Exportar** a JSON o Markdown.
2. **Prompt** completo, con botón de copiar.
3. **Configuración A | B** en dos columnas:
   - CLI y su versión, proveedor, modelo, esfuerzo, modo, límites;
   - preset con enlace a su copia guardada (la del momento de la corrida, no la actual) y su hash;
   - perfil del proyecto y hash de la copia.
4. **Resultado:** tabla A/B con estado final, motivo de fin, duración (agente y espera humana), tokens por categoría, coste estimado y confirmado, tests, archivos y líneas cambiadas, y hallazgos por severidad. Barras comparativas de coste, tokens y duración. Precio de models.dev aplicado en cada lado (instantánea con su fecha).
5. **Informe:**
   - conclusiones del evaluador comparativo;
   - análisis por lado;
   - hallazgos del revisor, con enlace a archivo y línea en el diff;
   - coste del propio informe;
   - si aún no existe, un botón para generarlo.
6. **Artefactos por lado,** con las mismas pestañas que en ejecución:
   - **Terminal:** reproducción del log de la PTY con controles de velocidad;
   - **Logs;**
   - **Cambios:** diff de solución y diff de harness;
   - **Tests:** salida completa;
   - **Eventos:** línea de tiempo del JSONL;
   - **Preview:** fase 2.

### `/settings` — Ajustes

- **Estado de las API keys**, leídas del `.env`: configurada o no, sin mostrar su valor. Botón para probar la conexión a través del proxy.
- **Límites por defecto** de cada lado: cada uno con su interruptor, todos **desactivados** por defecto.
- **Informe:** modelo que lo genera y opción **Generar automáticamente** al terminar la comparación (desactivada por defecto).
- **Lista de exclusiones** de archivos de harness.
- **Política de retención** y uso actual de disco.
- **Versiones fijadas** de cada CLI.
- **Modelos locales** (fase 2): URL del servidor local (por ejemplo, `http://host.docker.internal:11434/v1` para Ollama), botón para probar la conexión y lista de modelos detectados.

## Arquitectura

### Stack local

Docker Compose levanta dos servicios. El detalle de cada pieza está en [Stack técnico](#stack-técnico).

- **`api` (Go).** Sirve:
  - el frontend compilado;
  - la API REST;
  - SSE para los eventos;
  - WebSocket para las terminales;
  - el proxy de inferencia, en un puerto aparte solo accesible desde la red interna.

  También orquesta los contenedores.
- **`postgres`.** Base de datos.
- **Volúmenes persistentes** para la base de datos, perfiles de proyecto, presets (fase 2) y artefactos (diffs, grabaciones de terminal, logs, informes).
- **Acceso a Docker** por el socket del host, montado solo en `api`. Los contenedores de agente nunca lo ven.

**Requisitos del host:** Docker y Docker Compose. Go, Node.js y los CLIs no hacen falta en el host.

### Acceso a la ruta del proyecto

El servicio `app` corre dentro de Docker y no ve `C:\...` directamente. Para copiar el proyecto:

1. `app` pide al daemon de Docker un **contenedor auxiliar** que monta la ruta del host en solo lectura (`<ruta>:/src:ro`) y un volumen de staging.
2. El contenedor auxiliar copia al staging los archivos seleccionados (ver *Copia del proyecto*).
3. `app` construye las imágenes usando el staging como contexto.

El proyecto original nunca se modifica.

### Copia del proyecto

- **Si el proyecto es un repositorio Git**, se copian los archivos seguidos más los no seguidos que no estén ignorados (`git ls-files --cached --others --exclude-standard`, sin los borrados). Esto incluye los cambios sin commit: la copia refleja la carpeta **tal cual está**. Git se ejecuta en solo lectura y con hooks y `fsmonitor` desactivados.
- **Si no es un repositorio Git**, se copia todo excepto una lista por defecto (`node_modules`, `dist`, `build`, `.venv`, `target`, etc.), editable en el perfil.
- **No se copia el `.git` original.** La copia no tiene remotos, así que el agente no puede hacer push a GitLab. A cambio, el agente no ve el historial del proyecto (`git log`, `blame`).
- **`.env*` se excluye por defecto**, porque su contenido acabaría enviado al proveedor. El perfil puede incluirlo explícitamente.
- **Archivos de harness.** En la **fase 1** se copian tal cual: cada lado usa el harness que ya tiene el proyecto, y si no tiene, corre sin él. **A partir de la fase 2**, con presets, se excluyen en la raíz y anidados:
  - `AGENTS.md`, `CLAUDE.md`, `CLAUDE.local.md`, `GEMINI.md`;
  - `.claude/`, `.agents/`, `.codex/`, `.opencode/`, `opencode.json`, `opencode.jsonc`, `.mcp.json`;
  - `.cursor/`, `.cursorrules`, `.github/copilot-instructions.md`.

  La lista vive en configuración y se puede ampliar.
- **Límite de tamaño configurable.** Si la copia lo supera, se avisa antes de construir.

### Perfil de proyecto

Se guarda por ruta y contiene:

- **Runtime base:** una imagen como `node:22` o `python:3.12`, o el `Dockerfile` o `devcontainer.json` del propio proyecto si existe.
- **Comando de setup,** por ejemplo `npm ci`. Se ejecuta **dentro de la imagen**: las dependencias instaladas en Windows no sirven en un contenedor Linux, y `node_modules` no se copia.
- **Comando de tests/build/lint**, opcional.
- **Exclusiones adicionales.**
- **Comando y puerto de preview**, opcional (fase 2).

### Construcción de imágenes

Las imágenes se construyen por capas para que los dos lados partan exactamente del mismo estado y la caché de Docker acelere las repeticiones:

```
runtime base (perfil)
 └─ capa proyecto: copia + setup            ← compartida por A y B
     ├─ capa lado A: CLI A + preset A + baseline Git
     └─ capa lado B: CLI B + preset B + baseline Git
```

- **Las imágenes de CLI están versionadas.** La versión exacta de cada CLI se fija y se registra en la corrida.
- **Ninguna capa contiene API keys.**
- **La preparación (copia, setup y build) no cuenta** en el tiempo de la corrida. Se registra aparte.

### Línea base Git y diff limpio

En la capa de cada lado, después de colocar el preset:

1. `git init`, con `core.autocrlf=false` para que el diff refleje los bytes reales y los finales de línea de Windows no generen ruido.
2. `git add -A` y `git commit -m baseline`.

La línea base contiene los cambios sin commit del proyecto, sin el harness original y con el preset. Al terminar la corrida:

```bash
git add -A
git diff --cached baseline -- . ':(exclude)AGENTS.md' ':(exclude)CLAUDE.md' ':(exclude).claude' ':(exclude).agents' ...
```

Las exclusiones del diff salen de la misma lista de archivos de harness que usa la copia.

- **Diff de solución:** lo que el agente cambió en el código. Es lo que ve el revisor y lo que aparece en el informe.
- **Diff de harness:** los cambios que el agente haga en sus propios archivos (por ejemplo, si escribe en `CLAUDE.md`). Se muestra en una pestaña aparte.

### Ejecución y terminales

- **Un contenedor por lado**, creado a partir de su imagen, con la misma cuota de CPU y memoria en ambos lados. El usuario del contenedor no es root.
- **Cada contenedor tiene una PTY** retransmitida por WebSocket a una terminal xterm.js. La terminal admite entrada, salida ANSI, redimensionado y teclas de control. Sus comandos operan dentro del contenedor, nunca en el host.
- **El prompt inicial se envía según el adaptador del CLI:**
  - **Autónomo:** el prompt va como argumento del modo no interactivo del CLI, con los permisos en modo sin confirmaciones. El contenedor es la frontera de seguridad; no se intenta mantener una lista de comandos permitidos.
  - **Interactivo:** el CLI arranca en su TUI con el prompt inicial como argumento, si el CLI lo admite. Si no, el runner lo escribe en la PTY cuando la TUI está lista.

  La forma exacta de cada CLI se valida en el spike.
- **Esfuerzo:** cada adaptador traduce el valor a la opción de su CLI. Si un CLI o modelo no la admite, el control se deshabilita en el panel.
- **Límites por lado, todos opcionales:** timeout, tokens máximos y coste máximo. Sin límites, el lado corre hasta que el CLI termina o el usuario lo finaliza o cancela; el proxy sigue midiendo igual. Con límites, el proxy aplica los de tokens y coste cortando las peticiones y marcando el lado como «límite alcanzado».

### Proxy de inferencia (medición y claves)

Medir el coste es el objetivo principal, y en modo interactivo la TUI no entrega el uso de forma estructurada. Por eso todo el tráfico al proveedor pasa por un **proxy de inferencia propio, escrito en Go dentro de `api`**. El detalle técnico está en [Proxy de inferencia](#proxy-de-inferencia-go).

- **Cada lado recibe una URL base** que apunta al proxy y un **token de lado**. **La API key real nunca entra en el contenedor.**
- **No traduce formatos.** Cada CLI habla la API nativa de su proveedor (ver [Compatibilidad CLI ↔ proveedor](#compatibilidad-cli--proveedor)), así que el proxy solo reenvía.
- **Por cada petición registra** el uso (input, lectura de caché, escritura de caché, output), la latencia y los errores, atribuidos al lado por su token.
- **Los mismos datos alimentan** el coste y los tokens en vivo del dashboard y, si están activados, los límites de coste y tokens: al superarlos, el proxy rechaza las peticiones del lado y lo marca como «límite alcanzado».
- **Contraste:** al terminar se leen también los archivos de sesión del CLI dentro del contenedor, para contrastar el uso y extraer eventos (comandos, archivos tocados). Si no coinciden, prevalece el proxy y la discrepancia se anota.

Las API keys se definen en `ai-compare/.env`, creado a partir de `.env.example` e incluido en `.gitignore`. La UI solo indica si cada clave está configurada.

### Fin de corrida y verificación

1. El lado llega a un estado terminal: éxito, error, cancelado o límite alcanzado. **Los errores de infraestructura** (build, arranque, proxy) **se registran aparte** de los errores del agente.
2. El runner extrae los dos diffs, los logs de la PTY y los archivos de sesión del CLI.
3. **Si el perfil define un comando de tests**, el runner lo ejecuta en un **contenedor nuevo** creado desde la imagen del lado con el diff aplicado, no en el contenedor donde trabajó el agente.
4. **Tests ocultos opcionales:** el usuario puede indicar una carpeta de tests del host que el agente nunca ve. Se copia solo en la verificación, así que el agente no puede adaptarlos a su solución.
5. El contenedor del agente se **detiene pero no se borra** hasta que se aplique la política de retención. Eso permite relanzar la preview más adelante (fase 2).

### Reconexión y recuperación

**El navegador es solo un visor.** Toda la comparación vive en el backend: los contenedores, el orquestador, el proxy que mide y la grabación de cada terminal. Cerrar la pestaña no detiene nada.

**Si cierras la pestaña o se cae la conexión:**

1. Los contenedores siguen corriendo, el proxy sigue midiendo y el orquestador sigue aplicando límites y timeouts.
2. En modo interactivo, si el agente hace una pregunta mientras no hay nadie mirando, espera tu respuesta. El tiempo esperando cuenta como espera humana, no como tiempo del agente.
3. Al volver a abrir ai-compare, la barra superior muestra «comparación en curso» y `/` abre directamente la vista de ejecución.
4. Cada terminal se recupera así:
   - el WebSocket se reconecta solo, con reintentos;
   - el backend envía la salida acumulada desde el último borrado de pantalla, para que xterm.js reconstruya lo que se veía;
   - el navegador envía su tamaño de terminal y el backend lo aplica al contenedor. Ese cambio de tamaño hace que la TUI de `opencode` se redibuje entera, así que la pantalla queda exacta.
5. Las métricas y estados se recuperan con SSE (`Last-Event-ID`) y TanStack Query vuelve a pedir el estado actual.

**Varias pestañas** sobre la misma comparación funcionan a la vez: todas ven la misma terminal y cualquiera puede escribir.

**Si se reinicia el backend** (`api`):

- **Al arrancar,** el orquestador busca los contenedores por etiqueta y vuelve a engancharse a los que siguen vivos.
- **La salida producida mientras `api` estaba caído no se pierde.** Docker guarda la salida de cada contenedor en sus logs, también con TTY. La grabación se completa con `docker logs --since <último instante grabado>`.
- **El proxy** vuelve con el backend. Las peticiones que el CLI intente durante la caída fallan y el CLI las reintenta; se registran como errores de infraestructura.

**Lo que no se puede recuperar:** si se reinicia Docker Desktop o el equipo, los contenedores se detienen y su proceso se pierde. Esos lados quedan como error de infraestructura, con todo lo registrado hasta ese momento (diff, uso, grabación).

## Presets

**Fase 2.** En la fase 1 cada lado usa el harness que ya tiene el proyecto.

Un preset es un conjunto reutilizable de archivos de harness. Vive **en disco**, dentro del volumen de datos de ai-compare:

```
harnesses/
  mi-preset/
    preset.md        ← frontmatter: title, description, clis compatibles
    project/         ← espejo literal de lo que se copia a la raíz del proyecto
      AGENTS.md
      CLAUDE.md
      .claude/skills/...
      .agents/...
      .mcp.json
    home/            ← se copia al home del usuario del contenedor
      .codex/config.toml
```

- **`project/` es un espejo literal**, sin traducción entre formatos. Un preset puede llevar archivos para varios CLIs a la vez (`CLAUDE.md` y `AGENTS.md`, `.claude/` y `.agents/`), y cada CLI lee lo que entiende.
- **`home/` existe** porque parte de la configuración no vive en el proyecto. Por ejemplo, Codex lee sus MCP de `~/.codex/config.toml`.
- **Los secretos de MCP** van como referencias a variables de entorno del `.env`, nunca escritos en el preset.
- **`preset.md`:** su frontmatter tiene título, descripción y CLIs compatibles; el cuerpo son notas libres. Título y descripción son lo que ve el usuario en la lista.
- **En cada corrida** se guarda una copia del preset usado y su hash. Editar un preset después no cambia lo que explica un resultado antiguo.
- **Sin harness:** preset fijo del sistema. El lado corre sin ningún archivo de harness: se quitan los del proyecto y no se añade nada. Sirve para medir el modelo y el CLI «de serie».

La interfaz para gestionar presets se describe en [`/harnesses`](#harnesses--presets).

Fuera del MVP: cards como mecanismo de entrada con traducción automática a cada CLI, para usar un mismo preset «lógico» en claude, codex y opencode.

## Métricas

Por lado:

- **Configuración:** CLI y su versión, proveedor, modelo, esfuerzo, modo, preset con su hash, ruta del proyecto y hash de la copia.
- **Tokens por categoría:** input, lectura de caché, escritura de caché y output (el razonamiento se factura como output). Lo que no se pueda medir se marca «no reportado», nunca cero.
- **Coste estimado** con el precio de models.dev guardado al iniciar la comparación, y **coste confirmado** por el proveedor cuando esté disponible.
- **Velocidad:** tokens de output por segundo. Es especialmente útil con modelos locales, donde el coste es cero y lo que se compara es tiempo y calidad.
- **Tiempos:**
  - preparación (no comparable, informativa);
  - tiempo del agente;
  - en modo interactivo, **tiempo de espera humana**, estimado como los intervalos sin peticiones en curso que terminan con entrada del usuario.
- **Peticiones, reintentos y errores** del proveedor.
- **Tests:** resultado del comando del perfil y de los tests ocultos.
- **Diff:** diff de solución y diff de harness; archivos cambiados y líneas.
- **Hallazgos del revisor** por severidad.

### Precios

- **Fuente única: models.dev.** No hay tabla de precios propia. `api` descarga `https://models.dev/api.json` y la página [`/pricing`](#pricing--precios-solo-lectura) la muestra tal cual.
- **Caché local.** `api` guarda la última respuesta en un archivo del volumen de datos, con la fecha de descarga, y la refresca cada 24 h o al pulsar **Actualizar**. Si models.dev no responde, se usa la caché y se indica su fecha. Si no hay caché ni conexión, la comparación puede arrancar igual, con el coste «no calculable».
- **Instantánea por comparación.** Al pulsar **Ejecutar comparación**, se copia el objeto `cost` completo de cada modelo usado en la tabla `price_snapshots`, ligado a ese lado, con la fecha de descarga de models.dev. El coste de esa comparación siempre se calcula con esa instantánea, aunque models.dev cambie después.
- **Catálogo de modelos.** Los selects de modelo también salen de models.dev, filtrados por proveedor. Así la lista de modelos y sus precios nunca se desincronizan.
- **Modelo sin precio en models.dev:** su coste sale «no calculable», pero los tokens se registran.
- **Modelos locales:** coste cero por definición, marcado como «local», no como «no calculable».
- **Limitación aceptada:** no hay forma de corregir un precio a mano. Si models.dev tiene un error, el coste estimado lo hereda; el coste confirmado por el proveedor, cuando existe, sirve de contraste.

## Informe

Se genera al pulsar **Generar informe**, o automáticamente si está activada esa opción en `/settings`. Las etapas usan un modelo configurable, y su coste se registra aparte del de la comparación.

**Para no esperar al final:** las etapas que solo dependen de un lado (la revisión de su diff y su análisis) arrancan en cuanto ese lado termina, en segundo plano. Al terminar el segundo lado, solo queda el evaluador comparativo. Los diffs se etiquetan A/B para el revisor desde el principio, así que la revisión a ciegas se mantiene.

1. **Revisor de código, a ciegas.** Recibe los diffs etiquetados A y B en orden aleatorio, sin modelo, CLI ni preset. Busca bugs, fallos funcionales, casos límite, seguridad y problemas de tests; no puntúa estilo. Cada hallazgo lleva archivo y línea, impacto y severidad. Puede no encontrar nada.
2. **Analista por lado.** Resume qué ocurrió a partir de eventos, errores, comandos, uso, tests y diff, en un máximo de cinco párrafos, separando hechos de inferencias.
3. **Evaluador comparativo.** Recibe lo anterior más las métricas y explica qué lado rindió mejor y en qué hay trade-offs. Puede señalar «menor coste», «menor duración» o «menos problemas» en vez de declarar un ganador único.

Si el modelo juez coincide con uno de los comparados, el informe lo advierte (sesgo de autopreferencia).

**Página del informe:**

- tabla A/B de métricas;
- conclusiones;
- barras comparativas de coste, tokens por categoría y duración;
- tests y hallazgos por severidad;
- diffs navegables y logs, con enlaces a eventos.

Cuando haya repeticiones, se añade un gráfico de coste frente a calidad.

## Historial y retención

- **La base de datos guarda por comparación:**
  - prompt y ruta del proyecto;
  - hash de la copia y perfil usado;
  - configuración de ambos lados, presets con sus hashes y versiones de CLI;
  - estados, timestamps, uso, instantánea de precios, costes e informes;
  - referencias a los artefactos.
- **Los artefactos** (diffs, logs de PTY, JSONL, sesiones del CLI, resultados de tests) se guardan en volúmenes persistentes.
- **Comparaciones antiguas:** las terminales se reproducen desde el log.
- **Retención configurable:** borra primero contenedores detenidos e imágenes, y conserva los artefactos y los informes.

## Seguridad

- **La UI escucha solo en `127.0.0.1`.** No hay login, pero al arrancar se genera un **token de sesión** que va en la URL, como en Jupyter.
- **HTTP y WebSockets validan el token y la cabecera `Origin`.** Sin esto, cualquier web abierta en el navegador podría conectarse a las terminales.
- **Contenedores de agente:**
  - usuario no root;
  - sin socket de Docker;
  - sin API keys;
  - cuotas de CPU y memoria.

  En el MVP tienen salida a internet, porque el agente puede necesitar instalar dependencias.
- **El proyecto original se monta en solo lectura** y solo en el contenedor auxiliar de copia.

## Validez de la comparación

- **Varianza.** Una corrida por lado tiene mucha varianza. El modelo de datos soporta **repeticiones** (lado × intento) desde el principio, aunque el MVP lance una por lado. El informe lo advierte cuando solo hay una.
- **Modo interactivo.** La intervención humana hace la comparación menos pareja. Esas comparaciones se marcan y el tiempo de espera humana se muestra aparte.
- **Límites de proveedor.** Si ambos lados usan el mismo proveedor en paralelo, comparten rate limits. Se registran los 429 y los reintentos para no confundirlos con lentitud del modelo.
- **Mismo punto de partida.** Ambos lados parten de la misma capa de proyecto: copia y setup idénticos.

## Stack técnico

```
navegador (React)
   │  REST (JSON) · SSE (eventos) · WebSocket (terminales)
   ▼
api (Go) ─────────────── postgres
   │  Docker Engine API (socket del host)
   ├──► contenedor auxiliar de copia ── monta la ruta del proyecto en solo lectura
   ├──► contenedor lado A ──┐
   └──► contenedor lado B ──┴──► proxy de inferencia (dentro de api, :4701)
                                     ├──► OpenAI (fase 1)
                                     ├──► Anthropic (fase 2)
                                     └──► servidor local del host: Ollama, LM Studio… (fase 2)

api ──► models.dev (catálogo de modelos y precios, con caché)
```

### Docker desde un contenedor

Sí funciona, y sin anidar Docker. El patrón se llama *Docker-out-of-Docker*:

1. El contenedor `api` monta el socket del daemon del host (`/var/run/docker.sock`).
2. Con ese socket, `api` le pide al **mismo daemon del host** que cree contenedores.

Los contenedores de cada lado no nacen «dentro» de `api`: son **hermanos** suyos en el mismo Docker. Docker Desktop en Windows y macOS expone el socket en esa misma ruta para los contenedores, así que el `docker-compose.yml` es igual en los tres sistemas.

Consecuencias que hay que tener en cuenta:

- **Rutas de bind mount.** Las resuelve el daemon del host, no `api`. Por eso `api` puede pedir montar `C:\Users\Jose\...` en el contenedor auxiliar de copia aunque `api` no vea esa ruta. Docker Desktop traduce las rutas de Windows.
- **Volúmenes con nombre** (`ai-compare_artifacts`, `ai-compare_staging`). Se comparten por nombre entre `api` y los contenedores que crea.
- **Red.** Compose crea la red `ai-compare` y los contenedores de lado se conectan a ella. Así llegan al proxy como `http://api:4701`. Ese puerto no se publica en el host, y la UI (`:4700`) no se expone a los contenedores de lado.
- **Etiquetas** en cada contenedor e imagen (`ai-compare.comparison=<id>`, `ai-compare.side=A`). Sirven para limpiar y para reconciliar el estado si `api` se reinicia.
- **Imágenes.** Se construyen con la API de build de Docker (BuildKit), usando como contexto el volumen de staging.
- **Descartado: Docker-in-Docker** (un daemon dentro de un contenedor `privileged`). Es más lento, tiene su propia caché de imágenes y exige más permisos.
- **Riesgo asumido.** Quien controla el socket controla el host. Por eso solo `api` lo monta, la UI escucha solo en `127.0.0.1` y exige token de sesión.

### Backend (Go)

Go encaja bien: el SDK oficial de Docker está escrito en Go, y el stream de la terminal y SSE son sencillos con la librería estándar.

| Necesidad | Elección |
|---|---|
| HTTP y rutas | `net/http` de la librería estándar (el enrutado con métodos y parámetros basta) |
| Contrato de la API | Especificación **OpenAPI** como fuente de verdad. `oapi-codegen` genera las interfaces del servidor Go y `openapi-typescript` los tipos del frontend. |
| Base de datos | `pgx` + `sqlc` (consultas SQL tipadas generadas) |
| Migraciones | `goose`, embebidas en el binario y aplicadas al arrancar |
| Docker | SDK oficial de Docker para Go (contenedores, build, attach con TTY) |
| Terminales | WebSocket con `github.com/coder/websocket`; se conecta al stream de `attach` del contenedor |
| Eventos | SSE con la librería estándar (`http.Flusher`) |
| Logs | `log/slog` en JSON |
| Configuración | Variables de entorno desde `.env` |
| Frontend en producción | Se embebe la build de Vite con `embed.FS`; un solo binario sirve todo |
| Desarrollo | `air` para recarga en caliente; Vite con proxy hacia `api` |
| Tests | `go test` y `testcontainers-go` para Postgres y Docker reales |

Estructura del repositorio:

```
ai-compare/
  api/openapi.yaml           contrato de la API
  backend/
    cmd/server/              main
    internal/
      http/                  handlers REST, SSE y WebSocket
      orchestrator/          máquina de estados de cada comparación y lado
      docker/                copia, build, contenedores, attach
      adapters/              codex, claude, opencode: comando, flags, rutas de sesión
      providers/             openai (fase 1), anthropic y openai-compatible/local (fase 2)
      proxy/                 proxy de inferencia: reenvío, uso, límites
      catalog/               cliente de models.dev con caché: modelos y precios
      store/                 consultas sqlc y migraciones
  frontend/                  Vite + React
  images/cli-base/           Dockerfile con los CLIs fijados
  docker-compose.yml
  .env.example
  Taskfile.yml               dev, gen (OpenAPI y sqlc), images, test
```

`adapters` y `providers` están separados para que añadir Anthropic sea registrar un proveedor más, sin tocar los adaptadores de CLI.

### Compatibilidad CLI ↔ proveedor

Cada CLI usa solo los proveedores que habla de forma nativa. No se traducen APIs.

| CLI | OpenAI | Anthropic | Modelos locales | Fase |
|---|---|---|---|---|
| `opencode` | Sí | Sí | Sí (API compatible con OpenAI) | 1 (OpenAI) · 2 (Anthropic y locales) |
| `codex` | Sí | No | No | 2 |
| `claude` | No | Sí | No | 2 |

El formulario solo ofrece las combinaciones válidas: al elegir el CLI se filtran los proveedores y, con ellos, los modelos.

### Orquestación: ¿Temporal?

**Decidido: no.** En su lugar, un orquestador propio con estado en Postgres:

- **Máquina de estados por lado:** `pendiente → copiando → construyendo → arrancando → en_curso → verificando → terminado`, más `error`, `cancelado` y `límite`. Cada transición se guarda en Postgres con su timestamp.
- **Timeouts** con `context` de Go.
- **Al arrancar, `api` reconcilia:** busca los contenedores por etiqueta, retoma los que siguen vivos y marca como error de infraestructura lo que no puede retomar.
- **Interfaz `Orchestrator`** para poder cambiar a Temporal más adelante sin tocar el resto.

Por qué no Temporal todavía:

- **Infraestructura y conceptos extra.** Añade un servidor (más su UI), workers y un modelo de programación con reglas propias (los workflows deben ser deterministas).
- **No cubre la pieza más larga.** Una comparación es sobre todo un contenedor interactivo con una terminal en vivo. Temporal no puede reanudar ese stream tras una caída; la reconciliación con Docker hace falta igualmente.
- **Volumen mínimo.** Un usuario y una o dos comparaciones a la vez.

**Cuándo compensaría:** con repeticiones N por lado en cola, comparaciones por lotes o informes largos con reintentos (fase 2 en adelante).

### Tiempo real

- **SSE en `/api/events`:**
  - cambios de estado;
  - métricas en vivo, cada ~2 s;
  - «comparación terminada» e «informe generado».

  Admite `Last-Event-ID` para no perder eventos al reconectar. En el frontend, cada evento actualiza o invalida la caché de TanStack Query.
- **WebSocket en `/api/comparisons/:id/sides/:side/terminal`:**
  - salida y teclas en binario;
  - redimensionado como mensaje de control JSON.

  SSE no sirve aquí: solo va del servidor al cliente, y la terminal necesita enviar teclas con baja latencia.
- **Grabación.** La salida de cada terminal se graba en formato asciicast v2 para reproducirla en el historial.

### Base de datos (PostgreSQL)

- **Tablas principales:**
  - `projects` (ruta y perfil);
  - `comparisons`;
  - `sides`;
  - `side_transitions`;
  - `requests` (una fila por petición que pasa por el proxy: uso, latencia, estado);
  - `price_snapshots` (el `cost` de models.dev de cada modelo usado, copiado al iniciar la comparación);
  - `reports`;
  - `findings`.

  **No hay tabla de precios:** el catálogo vive en models.dev y en su caché local.
- **Artefactos fuera de la base de datos.** Diffs, grabaciones, logs y salidas de tests van en archivos del volumen de artefactos; la base de datos guarda la ruta y los metadatos.

### Proxy de inferencia (Go)

Se descartó LiteLLM. Su motivo principal era traducir entre APIs, y ya no hace falta porque cada CLI usa la API nativa de su proveedor. El proxy vive dentro de `api`, en un puerto propio (`:4701`) que solo ven los contenedores de lado.

**Cómo funciona una petición:**

1. El CLI del lado llama a `http://api:4701/<proveedor>/...` con su **token de lado** como API key.
2. El proxy identifica comparación y lado por el token, comprueba los límites y sustituye el token por la API key real.
3. Reenvía la petición **sin modificar el cuerpo** (`httputil.ReverseProxy`).
4. Copia la respuesta al CLI tal cual, también en streaming, y la lee en paralelo para extraer el uso.
5. Guarda una fila en `requests` y emite el evento de métricas por SSE.

**Lectura del uso por tipo de proveedor:**

| Tipo | Dónde está el uso | Fase |
|---|---|---|
| `openai` | Campo `usage` de la respuesta; en streaming, en el evento final de la Responses API o en el último chunk de Chat Completions | 1 |
| `anthropic` | `usage` de `message_start` y `message_delta` en streaming | 2 |
| `openai-compatible` (local) | Campo `usage` si el servidor lo envía. Si no, se marca «no reportado» | 2 |

**Límites:**

- **Opcionales.** Si un lado no tiene límite de tokens ni de coste, el proxy solo mide y nunca rechaza peticiones por consumo.
- **Tokens y coste por lado:** el proxy suma el uso de cada respuesta. Al superar el límite, rechaza las siguientes peticiones con un error claro y el orquestador marca el lado como «límite alcanzado». La petición en curso termina; el exceso posible es de una respuesta.
- **Timeout:** lo aplica el orquestador, no el proxy.

**Seguridad:**

- Los tokens de lado son aleatorios y caducan al terminar la comparación.
- Las API keys reales solo están en el entorno de `api`.
- El proxy solo acepta rutas de los proveedores configurados; no es un proxy abierto.

### Modelos locales

`opencode` admite proveedores con API compatible con OpenAI. Los modelos locales entran por ahí: un tipo de proveedor más, `openai-compatible`, sin cambios en los adaptadores.

- **Dónde corre el modelo:** en el host, con Ollama, LM Studio, llama.cpp server o vLLM. El proxy lo alcanza en `http://host.docker.internal:<puerto>/v1`; Docker Desktop resuelve ese nombre al host. La URL se configura en `/settings`.
- **El tráfico también pasa por el proxy**, aunque no haya API key, para medir tokens y tiempos igual que con los proveedores de pago.
- **Lista de modelos:** sale del endpoint `/v1/models` del servidor local, porque models.dev no los conoce.
- **Coste:** cero, marcado como «local». Lo que se compara es tiempo, tokens por segundo y calidad.
- **Uso en streaming:** algunos servidores locales solo envían el uso si la petición lo pide (`stream_options.include_usage`). Si `opencode` no lo pide, el proxy tendría que añadirlo. Sería la única modificación del cuerpo de una petición, y solo para este tipo de proveedor. Se valida en el spike de la fase 2.
- **Recursos iguales para cada lado:**
  - **CPU y memoria de los contenedores:** sí se reparten por igual. Cada contenedor de lado tiene la misma cuota.
  - **La GPU de los modelos locales:** no se puede repartir. El modelo no corre en el contenedor del lado sino en el servidor local del host (Ollama, LM Studio…), que gestiona la GPU. En GPUs de consumo no existe una forma de partirla en dos mitades garantizadas.
- **Qué pasa en paralelo según la combinación:**

  | Lados | GPU compartida | Comportamiento |
  |---|---|---|
  | Remoto vs remoto | No | Paralelo, sin problema |
  | Local vs remoto | No (solo un lado usa la GPU) | Paralelo, sin problema |
  | Local vs local | Sí | Paralelo por defecto, con aviso |

- **Local contra local en paralelo:**
  - **Requisito:** que los dos modelos quepan a la vez en la memoria de la GPU (en Ollama, `OLLAMA_MAX_LOADED_MODELS` ≥ 2). Si no caben, el servidor los carga y descarga por turnos, y los tiempos se disparan de forma injusta.
  - **Antes de lanzar:** `api` consulta al servidor local los modelos cargados y su tamaño, y avisa si no caben juntos.
  - **Durante la corrida:** se mide en cada lado el tiempo hasta el primer token y los tokens por segundo. El informe marca los tiempos como «con GPU compartida».
- **En secuencia, opcional:** solo se ofrece cuando ambos lados son locales, para quien prefiera tiempos limpios aunque tarde más. Aun así, la preparación (copia y build) de los dos lados se hace en paralelo; solo se espera la ejecución del agente.

### Imágenes de agente

Capas de cada imagen:

1. **Runtime del proyecto** (perfil).
2. **CLIs fijados:** `codex`, `claude` y `opencode`.
3. **Copia del proyecto y setup.**
4. **Capa del lado:** configuración del CLI apuntando al proxy y baseline Git.

Notas:

- Los CLIs se instalan como binarios autocontenidos cuando el CLI los ofrece. Si alguno necesita Node.js y el runtime del proyecto no lo trae (por ejemplo, `python:3.12`), se copia Node desde una etapa multi-stage.
- La capa de CLIs se cachea por runtime.
- Usuario no root, con `git` y `ripgrep` instalados.

### Frontend

| Necesidad | Elección |
|---|---|
| Base | Vite + React + TypeScript + Tailwind + shadcn/ui |
| Estado del servidor | **TanStack Query.** Sí hace falta: historial, catálogo de modelos y precios, y estado de comparaciones, actualizados por los eventos SSE. |
| Rutas | TanStack Router: rutas tipadas y filtros del historial en la URL |
| Cliente de la API | `openapi-fetch` con los tipos generados del contrato |
| Formularios | react-hook-form + zod (el componente Form de shadcn) |
| Terminal | `@xterm/xterm` + `@xterm/addon-fit` |
| Diffs | `react-diff-view` |
| Markdown | `react-markdown` + `remark-gfm` |
| Gráficas | Componentes chart de shadcn (Recharts) |
| Tests | Vitest + Testing Library; Playwright para el flujo completo al final de la fase 1 |

### Servicios de Docker Compose

| Servicio | Imagen | Puerto | Volúmenes |
|---|---|---|---|
| `api` | build local (Go + frontend embebido) | `127.0.0.1:4700` | socket de Docker, artefactos, staging |
| `postgres` | `postgres:17` | interno | datos |

`api` publica `:4700` (UI y API) solo en `127.0.0.1`. Su segundo puerto, `:4701` (proxy de inferencia), solo es accesible desde la red `ai-compare`.

Los contenedores de copia y de cada lado los crea `api` dinámicamente; no están en el compose.

## Alcance por fases

### Fase 0 — Stack y spike

**Esqueleto:**

- `docker-compose.yml` con `api` y `postgres`.
- Servidor Go con health check y migraciones.
- Frontend Vite con shadcn y la barra de navegación.
- Pipeline de generación: OpenAPI → Go y TypeScript, más sqlc.
- `Taskfile` con `dev`, `gen`, `images` y `test`.

**Spike.** Validar en Windows con Docker Desktop:

1. Desde `api`, crear el contenedor auxiliar que monta una ruta de Windows y copia el proyecto al staging.
2. Construir la imagen del lado desde el staging.
3. Arrancar el contenedor en la red `ai-compare` y comprobar que llega al proxy (`api:4701`) y a nada más del stack.
4. Hacer `attach` con TTY y retransmitirlo por WebSocket a una página mínima con xterm.js. Debe funcionar escribir, redimensionar y Ctrl+C.
5. Lanzar `opencode` con un modelo de OpenAI a través del proxy, en modo interactivo y autónomo.
6. Extraer en el proxy el uso de cada respuesta, con y sin streaming, y calcular el coste con el precio de models.dev.
7. Comprobar que `opencode` puede apuntar a `host.docker.internal`, como adelanto de los modelos locales.

**Criterio de salida:** los siete puntos funcionan, o sabemos exactamente cuál no y qué alternativa usar.

### Fase 1 — Comparación de modelos de OpenAI con el harness del proyecto

**Alcance:**

- **CLI:** solo **`opencode`**, en los dos lados. Es el único que habla tanto con OpenAI como con Anthropic, así que la fase 2 solo añade un proveedor. El selector de CLI existe en la UI, pero solo ofrece `opencode`; los adaptadores de `codex` y `claude` llegan en la fase 2.
- **Proveedor:** solo **OpenAI**. Los modelos se ofrecen desde un registro de proveedores, preparado para añadir Anthropic sin cambiar los adaptadores.
- **Harness:** cada lado usa **el harness que ya tiene el proyecto**, copiado tal cual. Si el proyecto no tiene, corre sin él. No hay presets ni página `/harnesses`. Como ambos lados usan el mismo CLI y la misma copia, reciben exactamente las mismas instrucciones. La UI lista los archivos de harness detectados e indica cuáles lee `opencode`.
- **Por lado se elige:**
  - modelo de OpenAI;
  - esfuerzo;
  - modo (interactivo o autónomo).

  Lo que se compara en esta fase es **modelo contra modelo**, o el mismo modelo con distinto esfuerzo o modo.
- **El diff de solución sigue excluyendo los archivos de harness**, para no mezclar cambios en `AGENTS.md` con el código.

**Fase 1a — Lanzar y ver:**

- `/`: ruta con su perfil, prompt y dos lados.
- Copia, capas y baseline.
- Paneles lado a lado con Terminal y Logs, en modo interactivo y autónomo.
- Finalizar y cancelar cada lado.
- Reconexión: cerrar y reabrir la pestaña recupera las terminales y el estado; reinicio de `api` con reenganche a los contenedores.

**Fase 1b — Medir y comparar:**

- Pestañas Cambios y Métricas, con coste y tokens en vivo y límites por presupuesto.
- `/pricing` de solo lectura con los modelos de OpenAI de models.dev, e instantánea de precios por comparación.
- `/history` y `/history/:id` con grabación de la terminal, diffs y tests.
- Verificación con tests en un contenedor nuevo.
- Informe: revisor a ciegas, analista y evaluador.
- `/settings`.

### Fase 2

- Proveedor Anthropic para `opencode`.
- Modelos locales para `opencode` (proveedor `openai-compatible`), en paralelo por defecto, con aviso de GPU compartida y opción de secuencia.
- CLIs `claude` (solo Anthropic) y `codex` (solo OpenAI). Con ellos aparece el aviso de que cada CLI lee archivos de harness distintos (`CLAUDE.md` y `.claude/` frente a `AGENTS.md`).
- Presets: página `/harnesses`, opción **Sin harness** y exclusión del harness del proyecto.
- Previews de las aplicaciones: proxy por subdominio (`a-<id>.localhost`) y relanzamiento desde contenedores detenidos.
- Repeticiones N por lado con agregados y el gráfico de coste frente a calidad.
- Asesor de presets: qué diferencias entre presets pudieron influir y qué cambiar.
- Cards de preset como mecanismo de entrada con traducción entre CLIs.

## Supuestos a validar en el spike

- `api` puede crear contenedores hermanos a través del socket en Docker Desktop para Windows, y montar rutas de Windows en el contenedor de copia.
- `opencode` acepta una URL base propia para OpenAI, de modo que todo su tráfico pase por el proxy. Lo mismo para `codex` y `claude` en la fase 2.
- El proxy reenvía sin alterar la API que usa `opencode` con OpenAI, incluido el streaming, y extrae el uso de cada respuesta.
- Los IDs de modelo de models.dev coinciden con los que acepta `opencode` para OpenAI.
- Los tres CLIs arrancan su TUI con un prompt inicial, o toleran que se escriba en la PTY.
- Los archivos de sesión de cada CLI son legibles y útiles para extraer eventos.
- La detección de runtime y setup cubre mis proyectos habituales.
- Construir las capas es lo bastante rápido con caché para no entorpecer el uso diario.

## Decisiones pendientes

- **Suscripciones.** Claude Pro/Max o ChatGPT en lugar de API key. El proxy no aplica igual y el coste no es por token. Propuesta: fuera del MVP, o soportado con coste «no aplicable» y tokens leídos de las sesiones del CLI.
- **Modelos de OpenAI** concretos de la fase 1. Por defecto, los que models.dev lista para `openai`.
- **Servidor local de referencia** para la fase 2: Ollama o LM Studio.
- **Valores sugeridos** al activar cada límite: timeout, tokens y coste por lado.
- **Retención por defecto** de contenedores, imágenes y artefactos.
- **Exportación y respaldo** de presets e historial: por ahora basta con copiar el volumen `harnesses/`.

**Decididas:**

- **Temporal:** fuera. Orquestador propio con estado en Postgres, detrás de una interfaz.
- **CLIs por proveedor:** `claude` solo con Anthropic, `codex` solo con OpenAI y `opencode` con ambos y con modelos locales. Fase 1 solo con `opencode` y OpenAI.
- **Pasarela:** proxy propio en Go dentro de `api`. LiteLLM descartado.
- **Precios:** sin tabla propia. models.dev es la fuente, con caché local; cada comparación guarda una instantánea de los precios que usó.

## Fuera de alcance

- Subir, fusionar o publicar el código generado. La copia no tiene remotos.
- Modificar el proyecto original o la configuración global de los CLIs del host.
- Multiusuario, cuentas, autenticación y despliegue alojado.
- Comparaciones masivas de muchos modelos a la vez.
- Afirmar causalidad entre una regla del preset y el resultado a partir de una sola corrida por lado.
