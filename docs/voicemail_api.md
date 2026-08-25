# API модуля голосовой почты

Модуль проксирует внутренний `vmapi` Asterisk через общий Go-шлюз.

Базовый публичный префикс:

```text
/spa/api/external-api/voicemail
```

Доступ снаружи защищён JWT микросервиса и ролью:

```text
ROLE_CITIZEN_APPEAL
```

Во внешний `vmapi` модуль ходит отдельным клиентом с `Authorization: Bearer`
из `VMAPI_TOKEN`. JSON-ответы наружу нормализуются в `camelCase`.

Важно: текущий `vmapi` отдаёт списком только новые обращения. Архивные записи
можно скачать только по известному `id` через audio-ручку. Списка архива,
фильтров по датам и полноценной пагинации upstream сейчас не предоставляет.

## Env

```env
VMAPI_URL=http://192.168.31.2:8090
VMAPI_TOKEN=
VMAPI_TIMEOUT_SECONDS=60
```

## GET /health

Проверяет доступность `vmapi` через Go-шлюз.

```bash
curl -H "Authorization: Bearer $JWT" \
  http://localhost:8087/spa/api/external-api/voicemail/health
```

Ответ:

```json
{
  "status": "ok",
  "mailboxes": 19
}
```

## GET /mailboxes

Возвращает сводку по всем голосовым ящикам.

```bash
curl -H "Authorization: Bearer $JWT" \
  http://localhost:8087/spa/api/external-api/voicemail/mailboxes
```

Ответ:

```json
{
  "totalNew": 3,
  "items": [
    {
      "mailbox": "090",
      "name": "Нерабочее время",
      "newCount": 3,
      "oldCount": 41
    },
    {
      "mailbox": "051",
      "name": "Отдел обращения граждан",
      "newCount": 0,
      "oldCount": 2
    }
  ]
}
```

Поля:

```text
totalNew  - сумма новых обращений по всем ящикам
items     - список ящиков
mailbox   - номер ящика для остальных ручек
name      - название отдела из vmapi
newCount  - новые, ещё не подтверждённые обращения
oldCount  - архивные или прослушанные с телефона обращения
```

## GET /mailboxes/{mailbox}/messages

Возвращает новые обращения конкретного ящика. Пока обращение не подтверждено
через `ack`, оно будет приходить повторно с тем же `id`.

Query-параметры:

```text
audio  - 1 по умолчанию у vmapi; 0 отдаёт только метаданные без base64 MP3
limit  - сколько новых обращений запросить у vmapi
```

Для интерфейса списка обычно использовать `audio=0`, а запись слушать отдельной
audio-ручкой по `audioUrl`.

```bash
curl -H "Authorization: Bearer $JWT" \
  "http://localhost:8087/spa/api/external-api/voicemail/mailboxes/090/messages?audio=0&limit=10"
```

Ответ без новых обращений:

```json
{
  "mailbox": "090",
  "count": 0,
  "truncated": false,
  "items": []
}
```

Ответ с обращением:

```json
{
  "mailbox": "090",
  "count": 1,
  "truncated": false,
  "items": [
    {
      "id": "1787591171-00000002",
      "mailbox": "090",
      "folder": "INBOX",
      "callerNumber": "+79495352139",
      "callerName": "",
      "receivedEpoch": 1787591171,
      "receivedAt": "2026-08-24T20:06:11+03:00",
      "durationSec": 9,
      "audio": null,
      "audioUrl": "/spa/api/external-api/voicemail/mailboxes/090/messages/1787591171-00000002/audio"
    }
  ]
}
```

Если запросить `audio=1` и `vmapi` успешно перекодировал запись, поле `audio`
будет объектом:

```json
{
  "format": "mp3",
  "bitrate": "32k",
  "sizeBytes": 39501,
  "base64": "SUQz..."
}
```

Если `vmapi` не смог перекодировать конкретную запись:

```json
{
  "audio": null,
  "audioError": "ffmpeg failed"
}
```

Поля сообщения:

```text
id             - идентификатор обращения; его передавать в ack
mailbox        - номер ящика
folder         - INBOX для новых обращений
callerNumber   - номер звонившего; может быть пустой строкой
callerName     - имя звонившего, если АТС его определила
receivedEpoch  - Unix-время звонка в секундах
receivedAt     - время звонка в RFC3339
durationSec    - длительность записи в секундах
audio          - MP3 в base64, если запрошен audio=1
audioUrl       - URL для прослушивания/скачивания MP3 отдельным запросом
truncated      - true, если новых обращений больше, чем отдал limit
```

`truncated` - это не полноценная пагинация. Рабочий цикл: забрать пачку,
создать задачи, подтвердить обработанные `ids` через `ack`, потом запросить
`messages` ещё раз и получить следующую пачку.

## GET /mailboxes/{mailbox}/messages/{id}/audio

Проксирует MP3-запись потоком. Работает по известному `id` и для новых, и для
уже подтверждённых записей, если `vmapi` может найти файл в архиве.

```bash
curl -H "Authorization: Bearer $JWT" \
  -o message.mp3 \
  http://localhost:8087/spa/api/external-api/voicemail/mailboxes/090/messages/1787591171-00000002/audio
```

Ответ:

```http
Content-Type: audio/mpeg
Cache-Control: no-store

<binary mp3>
```

Для браузера при Bearer JWT:

```js
const response = await fetch(audioUrl, {
  headers: { Authorization: `Bearer ${token}` },
});

const blob = await response.blob();
audio.src = URL.createObjectURL(blob);
```

Обычный `<audio src="...">` не сможет сам добавить `Authorization` header,
если фронт авторизуется Bearer-токеном, а не cookie.

## POST /mailboxes/{mailbox}/ack

Подтверждает обработку обращений. После успешного `ack` обращения исчезают из
списка новых `messages`, но MP3 остаётся в архиве `vmapi`.

Подтверждать нужно только те `ids`, по которым портал действительно создал
задачу или выполнил нужное действие. Пустой список `ids` допустим и ничего
не делает.

```bash
curl -X POST \
  -H "Authorization: Bearer $JWT" \
  -H "Content-Type: application/json" \
  -d '{"ids":["1787591171-00000002"]}' \
  http://localhost:8087/spa/api/external-api/voicemail/mailboxes/090/ack
```

Тело запроса:

```json
{
  "ids": ["1787591171-00000002", "1787591180-00000003"]
}
```

Ответ:

```json
{
  "mailbox": "090",
  "moved": ["1787591171-00000002"],
  "notFound": ["1787591180-00000003"]
}
```

Поля:

```text
moved     - что vmapi действительно перенёс из новых в архив
notFound  - чего среди новых уже не оказалось
```

`notFound` не считается фатальной ошибкой: сотрудник мог прослушать запись с
телефона, и Asterisk уже убрал её из новых.

## Ошибки

JSON-ошибки Go-шлюза:

```json
{"error": "текст ошибки"}
```

Основные статусы:

```text
400  - некорректный mailbox, тело JSON или отсутствует ids
401  - нет JWT или JWT не прошёл проверку
403  - JWT есть, но нет ROLE_CITIZEN_APPEAL
404  - vmapi не нашёл ящик или запись
409  - vmapi сообщает, что ящик занят; повторить через 1-2 секунды
502  - vmapi недоступен, неверный VMAPI_TOKEN, запрет по сети или ошибка upstream
```

Пример `409`:

```json
{
  "error": "mailbox busy"
}
```

Пример `502`:

```json
{
  "error": "Сервис голосовой почты недоступен (нет ответа)"
}
```
