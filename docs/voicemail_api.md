# API голосовой почты

Обращения, оставленные на автоответчик АТС. Модуль шлюза — сквозной прокси в
микросервис `go_voicemail_service_document_flow`: он проверяет JWT и роль,
подписывает запрос личностью пользователя и передаёт всё остальное как есть.
Тело ответа шлюз не разбирает, поэтому новые поля микросервиса появляются здесь
без правок в коде.

Записи забирает с АТС сам микросервис по ночному расписанию и хранит у себя.
Поэтому у обращений есть статус и комментарий администратора, работает пагинация
и фильтры, а прослушанное не пропадает из списка — на самой АТС ничего этого нет.

Базовый префикс:

```text
/spa/api/external-api/voicemail
```

## Доступ

JWT портала и роль `ROLE_CITIZEN_APPEAL` (её наследуют `ROLE_ADMIN` и
`ROLE_ANALYTIC`).

```bash
curl -H "Authorization: Bearer $JWT" \
  http://localhost:8081/spa/api/external-api/voicemail/messages
```

Разобрав токен, шлюз проставляет в проксируемый запрос два заголовка:

```text
X-User-Id    клейм id — им подписывается изменение статуса и комментария
X-User-Name  клейм username (логин; ФИО в токене нет)
```

Одноимённые заголовки, присланные клиентом, **затираются**. Микросервис своей
авторизации не имеет и верит им только потому, что живёт в закрытой сети
`voicemail-net`, куда пускают лишь шлюз.

Env шлюза:

```env
VOICEMAIL_SERVICE_URL=http://voicemail_service:8089
```

`VMAPI_URL` и `VMAPI_TOKEN` переехали в микросервис — шлюз про Asterisk больше
ничего не знает.

## Объект обращения

```json
{
  "id": 5,
  "recordId": "1787591175-00000005",
  "mailbox": "090",
  "mailboxName": "Нерабочее время",
  "callerNumber": "+79495352135",
  "callerName": "",
  "receivedAt": "2026-08-24T17:06:16Z",
  "durationSec": 14,
  "hasAudio": true,
  "status": "done",
  "adminComment": "перезвонил, вопрос решён",
  "updatedBy": { "id": 77, "name": "v.petrov" },
  "updatedAt": "2026-09-01T06:57:04Z"
}
```

| Поле | Тип | Значение |
|---|---|---|
| `id` | int | наш сквозной идентификатор, им адресуются все ручки |
| `recordId` | string | идентификатор записи на АТС; для поиска на самой станции, фронту как ключ не нужен |
| `mailbox` | string | номер ящика |
| `mailboxName` | string | название отдела на момент забора |
| `callerNumber` | string | номер звонившего; **пустая строка**, если абонент скрыл номер |
| `callerName` | string | имя, если АТС его определила; для внешних звонков обычно пусто |
| `receivedAt` | string | время звонка, RFC3339 в UTC |
| `durationSec` | int | длительность записи |
| `hasAudio` | bool | есть ли mp3; если `false`, ручка audio вернёт 404 |
| `audioError` | string | появляется только при `hasAudio: false` — причина |
| `status` | string | `new`, `in_progress`, `spam`, `done` |
| `adminComment` | string \| null | комментарий администратора |
| `updatedBy` | object \| null | кто последним менял статус или комментарий |
| `updatedAt` | string \| null | когда меняли, RFC3339 в UTC |

Ссылку на аудио фронт собирает сам: `{префикс}/messages/{id}/audio`.

Обращение, у которого не получилось перекодировать запись, приходит так —
метаданные на месте, звука нет:

```json
{
  "id": 3,
  "recordId": "1787591173-00000003",
  "mailbox": "090",
  "mailboxName": "Нерабочее время",
  "callerNumber": "+79495352133",
  "callerName": "",
  "receivedAt": "2026-08-24T17:06:14Z",
  "durationSec": 12,
  "hasAudio": false,
  "audioError": "vmapi: код 500: ffmpeg failed",
  "status": "new",
  "adminComment": null,
  "updatedBy": null,
  "updatedAt": null
}
```

Такое обращение микросервис не подтверждает на АТС и попробует дозабрать звук
следующей ночью. В интерфейсе стоит показать его без кнопки воспроизведения.

## GET /messages

Список обращений, новые сверху.

```bash
curl -H "Authorization: Bearer $JWT" \
  "http://localhost:8081/spa/api/external-api/voicemail/messages?page=1&limit=2"
```

```json
{
  "items": [
    {
      "id": 6,
      "recordId": "1787594412-00000007",
      "mailbox": "051",
      "mailboxName": "Отдел обращения граждан",
      "callerNumber": "",
      "callerName": "",
      "receivedAt": "2026-08-24T18:00:12Z",
      "durationSec": 31,
      "hasAudio": true,
      "status": "new",
      "adminComment": null,
      "updatedBy": null,
      "updatedAt": null
    },
    {
      "id": 5,
      "recordId": "1787591175-00000005",
      "mailbox": "090",
      "mailboxName": "Нерабочее время",
      "callerNumber": "+79495352135",
      "callerName": "",
      "receivedAt": "2026-08-24T17:06:16Z",
      "durationSec": 14,
      "hasAudio": true,
      "status": "done",
      "adminComment": "перезвонил, вопрос решён",
      "updatedBy": { "id": 77, "name": "v.petrov" },
      "updatedAt": "2026-09-01T06:57:04Z"
    }
  ],
  "pagination": { "page": 1, "limit": 2, "total": 6, "pages": 3 }
}
```

### Пагинация

| Параметр | По умолчанию | Значение |
|---|---|---|
| `page` | `1` | номер страницы, с единицы |
| `limit` | `20` | размер страницы, потолок `100` |

`page` меньше единицы, `limit` вне диапазона `1..100` или нечисловое значение —
не ошибка: подставляется значение по умолчанию.

Страница за пределами выборки возвращает пустой `items` и честную статистику,
а не 404:

```bash
GET /messages?page=4&limit=2
```
```json
{ "items": [], "pagination": { "page": 4, "limit": 2, "total": 6, "pages": 3 } }
```

| Поле | Значение |
|---|---|
| `total` | сколько всего обращений подходит под фильтр |
| `pages` | сколько страниц при текущем `limit`; `0`, если ничего не найдено |

### Фильтры

| Параметр | Пример | Значение |
|---|---|---|
| `status` | `new` | один из четырёх статусов |
| `mailbox` | `090` | номер ящика; список брать из `/mailboxes` |
| `dateFrom` | `2026-08-24` | с этой даты включительно |
| `dateTo` | `2026-08-24` | по эту дату включительно, до 23:59:59 |

Фильтры комбинируются через `И`.

```bash
# новые обращения ящика 090
GET /messages?mailbox=090&status=new&limit=50
→ { "pagination": { "total": 2, "pages": 1, ... } }

# всё за один день: dateTo расширяется до конца суток,
# поэтому одинаковые даты не дают пустую выборку
GET /messages?dateFrom=2026-08-24&dateTo=2026-08-24
→ { "pagination": { "total": 6, "pages": 1, ... } }

# ничего не найдено — не ошибка
GET /messages?dateFrom=2026-08-25
→ { "items": [], "pagination": { "total": 0, "pages": 0, ... } }
```

Даты принимаются и как `2026-08-24`, и как полный RFC3339
(`2026-08-24T17:00:00Z`). Пример с фильтром по статусу:

```bash
GET /messages?status=spam
```
```json
{
  "items": [
    {
      "id": 2,
      "recordId": "1787591172-00000002",
      "mailbox": "090",
      "mailboxName": "Нерабочее время",
      "callerNumber": "+79495352132",
      "callerName": "",
      "receivedAt": "2026-08-24T17:06:13Z",
      "durationSec": 11,
      "hasAudio": true,
      "status": "spam",
      "adminComment": "реклама пластиковых окон",
      "updatedBy": { "id": 77, "name": "Петров Владимир Алексеевич" },
      "updatedAt": "2026-09-01T06:57:04Z"
    }
  ],
  "pagination": { "page": 1, "limit": 20, "total": 1, "pages": 1 }
}
```

Несуществующий статус и неразобранная дата — 400, а не пустой список: опечатка
не должна выглядеть как «обращений нет».

```text
GET /messages?status=неведомый    → 400 {"error": "Недопустимый статус в фильтре"}
GET /messages?dateFrom=позавчера  → 400 {"error": "Некорректная дата в dateFrom"}
```

А вот кривые `page` и `limit` молча заменяются значениями по умолчанию — от них
выборка не «врёт», просто листается иначе:

```text
GET /messages?page=0&limit=500  → pagination: { page: 1, limit: 20, ... }
GET /messages?limit=abc         → pagination: { page: 1, limit: 20, ... }
```

## GET /mailboxes

Справочник ящиков для выпадающего фильтра. Собирается из самих обращений, на АТС
за ним никто не ходит — список работает, даже когда станция недоступна.

```bash
curl -H "Authorization: Bearer $JWT" \
  http://localhost:8081/spa/api/external-api/voicemail/mailboxes
```

```json
{
  "items": [
    { "mailbox": "051", "name": "Отдел обращения граждан", "total": 1 },
    { "mailbox": "090", "name": "Нерабочее время", "total": 5 }
  ]
}
```

`total` — сколько всего обращений накопилось в ящике.

Подписи у разных ящиков совпадают: «Договорной отдел» — это `152`, `352` и `752`
из трёх разных филиалов. **Показывать одно название недостаточно**, рядом нужен
номер ящика.

## GET /messages/{id}/audio

Отдаёт mp3 файлом.

```bash
curl -H "Authorization: Bearer $JWT" \
  -o obrashenie.mp3 \
  http://localhost:8081/spa/api/external-api/voicemail/messages/1/audio
```

```http
HTTP/1.1 200 OK
Content-Type: audio/mpeg
Accept-Ranges: bytes
Cache-Control: private, max-age=3600
```

Поддерживается `Range` — без него плеер в браузере не сможет перематывать запись:

```http
GET /messages/1/audio
Range: bytes=0-15

HTTP/1.1 206 Partial Content
Content-Range: bytes 0-15/31
```

Ошибки различаются намеренно:

```text
404  {"error": "Обращение не найдено"}            такого id нет
404  {"error": "У обращения нет записи разговора"} id есть, но hasAudio: false
```

Обычный `<audio src="...">` не подойдёт: браузер не добавит к нему заголовок
`Authorization`. Нужен fetch в blob:

```js
const response = await fetch(`${PREFIX}/messages/${id}/audio`, {
  headers: { Authorization: `Bearer ${token}` },
});
audio.src = URL.createObjectURL(await response.blob());
```

## PATCH /messages/{id}

Меняет статус и/или комментарий администратора. Оба поля необязательные, но хотя
бы одно должно присутствовать: не переданное поле не трогается.

```bash
curl -X PATCH \
  -H "Authorization: Bearer $JWT" \
  -H "Content-Type: application/json" \
  -d '{"status":"spam","adminComment":"реклама пластиковых окон"}' \
  http://localhost:8081/spa/api/external-api/voicemail/messages/2
```

| Поле | Обязательное | Значение |
|---|---|---|
| `status` | нет | `new`, `in_progress`, `spam`, `done` |
| `adminComment` | нет | текст комментария |
| `updatedByName` | нет | ФИО для показа, см. ниже |

Ответ — обращение целиком, уже изменённое:

```json
{
  "id": 1,
  "recordId": "1787591171-00000001",
  "mailbox": "090",
  "mailboxName": "Нерабочее время",
  "callerNumber": "+79495352131",
  "callerName": "",
  "receivedAt": "2026-08-24T17:06:12Z",
  "durationSec": 10,
  "hasAudio": true,
  "status": "new",
  "adminComment": "передано в абонотдел",
  "updatedBy": { "id": 92, "name": "a.ivanova" },
  "updatedAt": "2026-09-01T06:57:20Z"
}
```

Здесь менялся только комментарий — `status` остался прежним.

### Про `updatedByName`

В JWT портала лежит **логин**, а не ФИО: `username` — это поле `login`
пользователя. Поэтому по умолчанию в `updatedBy.name` попадёт `v.petrov`.

Если фронт передаст `updatedByName`, покажется оно:

```bash
-d '{"status":"spam","adminComment":"реклама","updatedByName":"Петров Владимир Алексеевич"}'
```
```json
"updatedBy": { "id": 77, "name": "Петров Владимир Алексеевич" }
```

ФИО текущего пользователя у фронта есть — он грузит его из `/spa/api/me`.

**`updatedBy.id` из тела не берётся никогда**, только из JWT. Поэтому подделка
имени безобидна: настоящая личность всегда в `id`, и по нему видно, кто это был.
Строка обрезается до 255 символов; пустая или отсутствующая — останется логин.

### Ошибки

```text
400  {"error": "Нечего менять: нужен status или adminComment"}   пустое тело
400  {"error": "Недопустимый статус"}                            статуса нет в списке
400  {"error": "Не передан идентификатор пользователя"}          не доехал X-User-Id
400  {"error": "Некорректное тело запроса"}                      битый JSON
404  {"error": "Обращение не найдено"}
```

Последний случай в норме не встречается: `X-User-Id` проставляет шлюз. Если он
пришёл пустым, значит запрос пришёл не через шлюз — тогда отказ, а не «аноним».

## GET /health

```bash
curl -H "Authorization: Bearer $JWT" \
  http://localhost:8081/spa/api/external-api/voicemail/health
```

```json
{
  "status": "ok",
  "service": "voicemail",
  "lastSyncAt": "2026-09-01T06:56:53Z",
  "pendingAck": 1
}
```

| Поле | Значение |
|---|---|
| `lastSyncAt` | когда последний раз что-то забиралось с АТС; `null`, если ни разу |
| `pendingAck` | сохранено у нас, но ещё висит в INBOX на АТС |

`pendingAck` больше нуля — обычно записи, у которых не скачался звук: их
намеренно не подтверждают, пока mp3 не доедет. Растущее число — повод посмотреть
логи микросервиса.

## Статусы

| Значение | Смысл |
|---|---|
| `new` | никто не разбирал |
| `in_progress` | взяли в работу |
| `spam` | реклама или ошиблись номером |
| `done` | обработано |

Стартовое значение — `new`, справочника в базе нет, набор зашит в схему.

## Коды ответов

| Код | Откуда | Когда |
|---|---|---|
| `200` | — | успех |
| `400` | микросервис | некорректные параметры или тело |
| `401` | шлюз | нет JWT или он не прошёл проверку |
| `403` | шлюз | JWT есть, но нет `ROLE_CITIZEN_APPEAL` |
| `404` | микросервис | нет такого обращения или у него нет записи |
| `500` | микросервис | ошибка базы |
| `502` | шлюз | микросервис недоступен или не настроен |

Тело ошибки везде одинаковое:

```json
{ "error": "текст ошибки" }
```

Всё, кроме 401, 403 и 502, приходит от микросервиса и проходит через шлюз без
изменений.
