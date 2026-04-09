# Work Activity Tracker

Утилита на Go для учета рабочего времени в Linux.

Текущая реализация собрана по архитектуре с разделением на:

- `cmd/...` для entrypoint под конкретную ОС/окружение;
- `internal/app` для orchestration;
- `internal/bootstrap` для общего startup-кода entrypoint'ов;
- `internal/tracker` для бизнес-логики;
- `internal/platform` для платформенных интерфейсов;
- `internal/platform/linuxx11` для Linux X11 / GNOME-специфики.

Сейчас реализован вариант только для `Linux X11`.

Подготовлены каркасы entrypoint'ов:

- `cmd/tracker-linux-x11`
- `cmd/tracker-tray-linux-x11`
- `cmd/tracker-linux-wayland`
- `cmd/tracker-macos`
- `cmd/tracker-windows`

И зарезервированы платформенные пакеты:

- `internal/platform/linuxx11`
- `internal/platform/linuxwayland`
- `internal/platform/macos`
- `internal/platform/windows`

## Что умеет

- считает **активное** время;
- считает **неактивное** время;
- останавливает учет при отсутствии активности;
- показывает desktop notification перед остановкой;
- останавливает учет при блокировке экрана;
- умеет Telegram-бота:
  - логи,
  - inline-кнопки,
  - старт/пауза,
  - продолжить день,
  - добавить время,
  - завершить день;
- умеет HTTP API;
- умеет встроенный web UI на том же HTTP сервере;
- сохраняет историю сессий в JSON;
- умеет продолжать предыдущий день по истории;
- умеет опционально писать весь консольный вывод в лог-файл;
- умеет опционально отключать системные desktop notifications;
- умеет отдельное tray-приложение через HTTP API;
- умеет отслеживать активное окно;
- умеет останавливать активность по совпадению подстрок из конфига в:
  - заголовке окна,
  - `_GTK_APPLICATION_ID`,
  - `_KDE_NET_WM_DESKTOP_FILE`,
  - `WM_CLASS`.

---

## Ограничения

Для отслеживания активного окна используются:

- `xdotool`
- `xprop`

Это обычно работает в **X11**.  
В **Wayland** получение активного окна может не работать.

---

## Зависимости

### Системные

Установить:

```bash
sudo apt update
sudo apt install -y libnotify-bin xdotool x11-utils
```

`x11-utils` нужен для `xprop`.
`libnotify-bin` нужен только если включены desktop notifications.

### Go

Рекомендуется Go 1.25+.

---

## go.mod

Пример:

```go
module work-activity-tracker

go 1.25

require (
    github.com/go-telegram-bot-api/telegram-bot-api/v5 v5.5.1
    github.com/godbus/dbus/v5 v5.2.2
)
```

---

## Установка зависимостей Go

```bash
go mod init work-activity-tracker
go get github.com/go-telegram-bot-api/telegram-bot-api/v5
go get github.com/godbus/dbus/v5
go mod tidy
```

---

## Сборка

```bash
go build -o work-activity-tracker ./cmd/tracker-linux-x11
```

Tray-приложение:

```bash
go build -o ./bin/work-activity-tracker-tray ./cmd/tracker-tray-linux-x11
```

---

## Конфиг

Если рядом с бинарником или в текущей директории есть `config.json`, он будет загружен автоматически.

Также можно явно указать:

```bash
./work-activity-tracker --config=config.json
./work-activity-tracker -config config.json
```

### Пример `config.json`

```json
{
  "telegram_token": "",
  "telegram_chat_id": 0,
  "http_port": 8080,
  "auto_start_day": true,
  "enable_desktop_notifications": true,
  "history_file": "session-history.json",
  "log_file": "",
  "idle_warn_after": "2m",
  "stop_after_warn": "1m",
  "poll_interval": "5s",
  "excluded_window_substrings": [
    "Telegram",
    "Youtube"
  ]
}
```

---

## Параметры конфига

* `telegram_token` — токен Telegram-бота. Если пустой, бот не запускается.
* `telegram_chat_id` — ID чата для сообщений и управления.
* `http_port` — порт HTTP API. Если `0`, API не запускается.
* `auto_start_day` — автоматически начинать новый день при старте программы. Если `false`, программа запускается в состоянии "день не начат".
* `enable_desktop_notifications` — включить системные уведомления перед автопаузой. Если `false`, уведомления через `notify-send` не показываются.
* `history_file` — путь к JSON-файлу истории сессий.
* `log_file` — путь к опциональному лог-файлу. Если пустой, лог в файл не пишется.
* `idle_warn_after` — время бездействия до предупреждения.
* `stop_after_warn` — время после предупреждения до остановки учета.
* `poll_interval` — интервал polling.
* `excluded_window_substrings` — список подстрок. Если хотя бы одна найдена в одном из полей окна, активность сразу останавливается.

Для tray-приложения используется отдельный конфиг `tray-config.json`.

Пример:

```json
{
  "api_base_url": "http://127.0.0.1:8080",
  "poll_interval": "5s",
  "request_timeout": "3s"
}
```

Поля:

* `api_base_url` — базовый URL HTTP API основного трекера.
* `poll_interval` — как часто tray обновляет статус.
* `request_timeout` — timeout HTTP-запросов tray-приложения.

---

## В каких полях окна идет поиск совпадений

При активном окне программа проверяет совпадения в:

* заголовке окна;
* `_GTK_APPLICATION_ID`;
* `_KDE_NET_WM_DESKTOP_FILE`;
* `WM_CLASS`.

Данные берутся из команды:

```bash
xprop -id $(xdotool getwindowfocus)
```

---

## Логика работы

### Активность

Активностью считается пользовательская активность по idle-монитору GNOME.

* пока есть активность — идет подсчет активного времени;
* если активности нет `idle_warn_after` — показывается уведомление, если `enable_desktop_notifications=true`;
* если активности нет еще `stop_after_warn` — учет останавливается.

### Блокировка экрана

Если экран заблокирован:

* учет активности останавливается сразу;
* идет счетчик неактивности.

### Исключенные окна

Если активное окно совпало по одному из полей с подстрокой из `excluded_window_substrings`:

* учет активности останавливается сразу;
* идет неактивное время;
* когда окно сменится на разрешенное, учет может снова стартовать.

---

## Что показывается в статусе

* старт сессии;
* состояние;
* итого активности;
* итого неактивности;
* вручную добавленное время;
* заголовок активного окна;
* `GTK_APPLICATION_ID`;
* `KDE_NET_WM_DESKTOP_FILE`;
* `WM_CLASS`.

---

## HTTP API

Если `http_port > 0`, доступны endpoints.

Web UI доступен по корневому адресу:

```bash
http://127.0.0.1:8080/
```

При старте HTTP сервера программа пишет эту ссылку в лог.

### Статус

```bash
curl http://127.0.0.1:8080/status
```

### Добавить время

```bash
curl "http://127.0.0.1:8080/add?minutes=90"
```

### Пауза

```bash
curl -X POST http://127.0.0.1:8080/pause
```

### Возобновить

```bash
curl -X POST http://127.0.0.1:8080/start
```

Если день еще не начат или уже завершен, этот endpoint начинает новый день.

### Начать новый день

```bash
curl -X POST http://127.0.0.1:8080/new-day
```

### Продолжить день

```bash
curl -X POST http://127.0.0.1:8080/continue-day
```

Этот endpoint восстанавливает предыдущий день, если последняя сессия началась сегодня или с момента ее завершения прошло меньше 6 часов.

### Завершить день

```bash
curl -X POST http://127.0.0.1:8080/end
```

### Вся история

```bash
curl http://127.0.0.1:8080/history
```

История хранится в JSON-файле, указанном в `history_file`.

---

## Web UI

Web UI отдается тем же HTTP сервером, что и API.

Что есть в интерфейсе:

* текущее состояние;
* активное, неактивное и добавленное время;
* информация об активном окне;
* кнопки для `start`, `pause`, `new day`, `continue day`, `add`, `end`;
* блок истории на основе `/history`.

---

## Telegram-бот

Если указаны `telegram_token` и `telegram_chat_id`, бот:

* отправляет и обновляет сообщение состояния;
* показывает inline-кнопки;
* отправляет логи;
* умеет добавлять время;
* умеет завершать день.

### Команды

* `/start`
* `/status`
* `/newday`
* `/continue`
* `/add 1h30m`
* `/pause`
* `/resume`
* `/end`

### Inline-кнопки

* `Пауза` / `Старт`
* `Начать новый день`
* `Продолжить день`
* `+30м`
* `+1ч`
* `+2ч`
* `Завершить день`

Кнопка `Продолжить день` появляется, если последняя сессия из истории началась сегодня или с момента ее завершения прошло меньше 6 часов.

---

## Tray-приложение

Tray-приложение запускается отдельно и подключается к HTTP API основного трекера.

По клику на иконку открывается меню с:

* активным временем;
* неактивным временем;
* текущим состоянием;
* действиями, повторяющими API: `refresh`, `start`, `pause`, `new day`, `continue day`, `add 30m`, `add 1h`, `add 2h`, `end`.

Иконка меняется по состоянию:

* зеленая — идет подсчет;
* серая — день не начат или завершен;
* желтая — пауза / ожидание / блокировка;
* красная — ошибка связи с API.

---

## Пример запуска

```bash
./work-activity-tracker
```

или

```bash
./work-activity-tracker --config=config.json
```

Основной трекер + tray:

```bash
./work-activity-tracker --config=config.json
./work-activity-tracker-tray --config=tray-config.json
```

---

## Systemd User Service

Для автозапуска трекера и tray есть готовый скрипт:

```bash
./scripts/install-systemd-user-services.sh
```

Что делает скрипт:

* собирает `./bin/work-activity-tracker`;
* собирает `./bin/work-activity-tracker-tray`;
* если нужно, создаёт `config.json` и `tray-config.json` из example-файлов;
* устанавливает `systemd --user` unit'ы;
* включает и запускает оба сервиса.

Установленные сервисы:

* `work-activity-tracker.service`
* `work-activity-tracker-tray.service`

Полезные команды:

```bash
systemctl --user status work-activity-tracker.service
systemctl --user status work-activity-tracker-tray.service
journalctl --user -u work-activity-tracker.service -f
journalctl --user -u work-activity-tracker-tray.service -f
```

---

## Полезная проверка вручную

### Заголовок активного окна

```bash
xdotool getwindowfocus getwindowname
```

### Полные свойства активного окна

```bash
xprop -id $(xdotool getwindowfocus)
```

Если там есть нужные поля и нужные строки, программа сможет по ним матчить.

---

## Типовая структура проекта

```text
.
├── cmd/tracker-linux-x11/main.go
├── go.mod
├── go.sum
└── config.json
```

---

## Возможные улучшения

* web UI;
* экспорт статистики по дням;
* fallback-механизмы для Wayland.
