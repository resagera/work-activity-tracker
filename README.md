# Work Activity Tracker

Утилита на Go для учета рабочего времени в Ubuntu/Linux.

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
  - добавить время,
  - завершить день;
- умеет HTTP API;
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
````

`x11-utils` нужен для `xprop`.

### Go

Рекомендуется Go 1.22+.

---

## go.mod

Пример:

```go
module work-activity-tracker

go 1.22

require (
    github.com/go-telegram-bot-api/telegram-bot-api/v5 v5.5.1
    github.com/godbus/dbus/v5 v5.1.0
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
go build -o work-activity-tracker
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
* `idle_warn_after` — время бездействия до предупреждения.
* `stop_after_warn` — время после предупреждения до остановки учета.
* `poll_interval` — интервал polling.
* `excluded_window_substrings` — список подстрок. Если хотя бы одна найдена в одном из полей окна, активность сразу останавливается.

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
* если активности нет `idle_warn_after` — показывается уведомление;
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
* заголовок активного окна;
* `GTK_APPLICATION_ID`;
* `KDE_NET_WM_DESKTOP_FILE`;
* `WM_CLASS`.

---

## HTTP API

Если `http_port > 0`, доступны endpoints.

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

### Завершить день

```bash
curl -X POST http://127.0.0.1:8080/end
```

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
* `/add 1h30m`
* `/pause`
* `/resume`
* `/end`

### Inline-кнопки

* `Пауза` / `Старт`
* `+30м`
* `+1ч`
* `+2ч`
* `Завершить день`

---

## Пример запуска

```bash
./work-activity-tracker
```

или

```bash
./work-activity-tracker --config=config.json
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
├── cmd/tracker/main.go
├── go.mod
├── go.sum
└── config.json
```

---

## Возможные улучшения

* хранение истории сессий в JSON/SQLite;
* systemd user service;
* web UI;
* экспорт статистики по дням;
* fallback-механизмы для Wayland.


