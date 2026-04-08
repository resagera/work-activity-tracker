# Work Activity Tracker

Утилита на Go для подсчета рабочего времени по активности пользователя в Ubuntu 22.04.

## Что умеет

* считает рабочее время по движению мыши и активности клавиатуры через idle-монитор GNOME;
* если активности нет заданное время, показывает desktop notification;
* если активности нет ещё заданное время после предупреждения, останавливает подсчет;
* при блокировке экрана сразу останавливает подсчет;
* при возвращении активности снова запускает подсчет;
* пишет события старта/остановки в stdout;
* может поднимать HTTP API;
* может отправлять логи и принимать команды через Telegram-бота;
* позволяет вручную добавить время к текущей сессии;
* позволяет завершить текущую сессию (день).

## Зависимости

### Системные

Нужно, чтобы в системе были:

* Ubuntu 22.04 с GNOME-сессией;
* `notify-send` для уведомлений;
* доступ к session D-Bus.

Установка `notify-send`:

```bash
sudo apt update
sudo apt install -y libnotify-bin xdotool
```

### Go

Минимально:

```bash
go version
```

Рекомендуется Go 1.22+.

## Go-модули

Пример `go.mod`:

```go
module work-activity-tracker

go 1.22

require (
    github.com/go-telegram-bot-api/telegram-bot-api/v5 v5.5.1
    github.com/godbus/dbus/v5 v5.1.0
)
```

Инициализация:

```bash
go mod init work-activity-tracker
go get github.com/go-telegram-bot-api/telegram-bot-api/v5
go get github.com/godbus/dbus/v5
go mod tidy
```

## Сборка

```bash
go build -o work-activity-tracker .
```

## Конфиг

Если рядом с бинарником или в текущей рабочей директории лежит `config.json`, он будет автоматически подхвачен даже без параметров запуска.

Также можно указать путь явно:

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
  "poll_interval": "5s"
}
```

### Параметры

* `telegram_token` — токен Telegram-бота. Если пустой, бот не запускается.
* `telegram_chat_id` — ID чата для отправки логов и управления.
* `http_port` — порт HTTP API. Если `0`, API не запускается.
* `idle_warn_after` — через сколько бездействия показать предупреждение.
* `stop_after_warn` — через сколько после предупреждения остановить подсчет.
* `poll_interval` — частота проверки idle и блокировки экрана.

## Аргументы запуска

Все параметры из конфига можно переопределить через CLI:

```bash
./work-activity-tracker \
  --http-port=8080 \
  --idle-warn-after=2m \
  --stop-after-warn=1m \
  --poll-interval=5s
```

Поддерживаются флаги:

* `-config`, `--config`
* `-telegram-token`, `--telegram-token`
* `-telegram-chat-id`, `--telegram-chat-id`
* `-http-port`, `--http-port`
* `-idle-warn-after`, `--idle-warn-after`
* `-stop-after-warn`, `--stop-after-warn`
* `-poll-interval`, `--poll-interval`

## Запуск

```bash
./work-activity-tracker
```

При старте программа:

* печатает итоговый конфиг в stdout;
* запускает HTTP API, если включен;
* запускает Telegram-бота, если задан токен;
* начинает отслеживать активность.

## HTTP API

Если `http_port > 0`, доступны endpoints:

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

## Telegram-бот

Если указаны `telegram_token` и `telegram_chat_id`, бот:

* отправляет сообщение о старте сессии;
* отправляет логи о старте/остановке учета;
* показывает inline-кнопку `Пауза` или `Старт`;
* позволяет добавить `30м`, `1ч`, `2ч`;
* позволяет завершить сессию.

### Команды

* `/start`
* `/status`
* `/add 1h30m`
* `/pause`
* `/resume`
* `/end`

## Логика работы

### Активность

Активность считается по idle-time GNOME:

* если idle почти нулевой, считается, что пользователь снова активен;
* если idle больше `idle_warn_after`, показывается предупреждение;
* если idle больше `idle_warn_after + stop_after_warn`, подсчет останавливается.

### Блокировка экрана

Если экран блокируется, подсчет останавливается сразу.

После разблокировки программа снова ждет активности и запускает учет.

## Ограничения

* ориентировано на Ubuntu 22.04 + GNOME;
* в других DE/WM D-Bus интерфейсы могут отличаться;
* если `org.gnome.Mutter.IdleMonitor` недоступен, idle-логика не сработает;
* если `org.gnome.ScreenSaver.Active` недоступен, определение блокировки может не работать.

## Пример структуры проекта

```text
.
├── main.go
├── go.mod
├── go.sum
└── config.json
```

## Что можно улучшить дальше

* сохранение истории сессий в JSON/SQLite;
* отдельная команда для старта нового дня;
* автозапуск через systemd user service;
* web UI вместо простого HTTP API;
* fallback на `loginctl` или `gdbus` для проверки lock state в нестандартных окружениях.

