# Gozon
## Полностью реализованная микросервисная система заказов и платежей с асинхронной обработкой через RabbitMQ, WebSocket уведомлениями и гарантией доставки сообщений (Inbox/Outbox паттерны).

---
## Архитектура

Система состоит из **4 независимых сервисов**, которые обмениваются сообщениями через **RabbitMQ** и синхронизируют данные через **PostgreSQL**:

```
┌──────────────────────────────────────────────────────────┐
│                    API Gateway (Port 8080)               │
│                Прокси + WebSocket сервер + CORS          │
└────────┬──────────────┬──────────────────────────────────┘
         │              │
    ┌────▼──────────┐   └──────────────────────┐
    │ Order Service │                          │
    │               │                  ┌───────▼────────────┐        
    │ (Port 8081)   │                  │   Payment Service  │
    │               │                  │   (Port 8082)      │
    │ - Создание    │                  │                    │
    │   заказов     │                  │ - Проверка баланса │
    │ - Outbox      │                  │ - Вычитание средств│
    │   паттерн     │                  │ - Outbox паттерн   │
    └────┬──────────┘                  └──────┬─────────────┘
         │                                    │
         │         ┌──────────────────────────┘
         │         │
         │    RabbitMQ (Port 5672)
         │    rabbitmq-management (Port 15672)
         │    
         ├────────► orders_queue ──────────────► Payment Service
         │           (выполнить платёж)
         │
         │◄───────── payment_events_fanout ◄───── Payment Service
         │           (результат: success/failed)
         │
         ▼
    PostgreSQL (Port 5435)
    - orders table
    - accounts table
    - outbox_events table
    - inbox_messages table
```

**Ключевая особенность:** Благодаря **Inbox/Outbox паттернам**, все сообщения гарантированно доставляются, даже если один из сервисов временно недоступен.

---

##Технологический стек

| Компонент           | Версия                | Назначение                               |
|---------------------|-----------------------|------------------------------------------|
| **Go**              | 1.25+                 | Backend (все микросервисы)               |
| **PostgreSQL**      | 15 Alpine             | Основная БД + Outbox/Inbox таблицы       |
| **RabbitMQ**        | 3 Alpine + Management | Message Broker для асинхронной обработки |
| **Docker**          | Latest                | Контейнеризация сервисов                 |
| **WebSocket**       | gorilla/websocket     | Real-time уведомления клиентам           |
| **OpenAPI/Swagger** | 3.0.0                 | Документация API                         |


## Структура проекта

```
gozon/
├── docker-compose.yml
├── README.md
├── api-gateway/
│   ├── cmd/
│   │   └── main.go                   # Точка входа Gateway
│   ├── openapi.yaml                  # Swagger документация
│   ├── go.mod
│   └── Dockerfile
├── deploy/
│   └── init.sql                      # Создание баз данных 
├── client/
│   └── index.html                    # Веб-интерфейс (HTML)
├── order-service/
│   ├── cmd/
│   │   └── main.go                   # Точка входа Order Service
│   ├── internal/
│   │   ├── domain/
│   │   │   └── models.go             # OrderStatus enum (new/finished/cancelled)
│   │   ├── handler/
│   │   │   └── order_handler.go      # HTTP endpoints
│   │   ├── repository/
│   │   │   └── order_repository.go   # Работа с БД + Outbox создание
│   │   ├── inbox/
│   │   │   └── processor.go          # Слушает payment_events_fanout
│   │   └── outbox/
│   │       └── processor.go          # Отправляет events в RabbitMQ
│   ├── go.mod
│   └── Dockerfile
└── payment-service/
    ├── cmd/
    │   └── main.go                   # Точка входа Payment Service
    ├── internal/
    │   ├── domain/
    │   │   └── models.go             # PaymentStatus enum (/success/failed)
    │   ├── handler/
    │   │   └── account_handler.go    # HTTP endpoints
    │   ├── repository/
    │   │   └── account_repository.go # Работа с БД
    │   ├── inbox/
    │   │   └── processor.go          # Слушает orders_queue
    │   └── outbox/
    │       └── processor.go          # Отправляет events в payment_events_fanout
    ├── go.mod
    └── Dockerfile
```

---

## Быстрый старт

### 1. Требования

- Docker & Docker Compose
- Git

### 2. Запуск

```bash
docker compose down -v

docker compose up --build
```

---

## API и Swagger

### Доступные адреса
**ОЧЕНЬ ВАЖНО!!!**
**Для доступа к веб-интерфейсу просто откройте файл client/index.html в любом браузере. Там написан небольшой HTML,
который позволяет протестировать простейший интерфейс, а также отправку пуш-уведомлений.**

| URL                          | Описание                                         |
|------------------------------|--------------------------------------------------|
| **http://localhost:8083**    | Swagger UI (интерактивная документация API)      |
| **http://localhost:15672**   | RabbitMQ Management (user/password)              |

### Тестирование API через Swagger

1. Открыть **http://localhost:8083** в браузере
2. Все endpoints будут интерактивны
3. Нажать **"Try it out"** и выполнять запросы прямо оттуда

### Основные endpoints

#### **Accounts (Платежи)**

```http
POST /accounts
{
  "user_id": 1
}
Response: 201 Created
{
  "id": 1,
  "user_id": 1,
  "balance": 0,
  "created_at": "2025-12-24T..."
}

POST /accounts/deposit
{
  "user_id": 1,
  "amount": 1000
}
Response: 200 OK
{
  "id": 1,
  "user_id": 1,
  "balance": 1000,
  "created_at": "2025-12-24T..."
}

GET /accounts/balance?user_id=1
Response: 200 OK
{
  "balance": 1000
}
```

#### **Orders (Заказы)**

```http
POST /orders
{
  "user_id": 1,
  "amount": 200
}
Response: 201 Created
{
  "id": 1,
  "user_id": 1,
  "amount": 200,
  "status": "new",
  "created_at": "2025-12-24T..."
}

GET /orders?user_id=1
Response: 200 OK
[
  {
    "id": 1,
    "user_id": 1,
    "amount": 200,
    "status": "finished",
    "created_at": "2025-12-24T..."
  }
]

GET /orders/by-id?id=1
Response: 200 OK
{
  "id": 1,
  "user_id": 1,
  "amount": 200,
  "status": "finished",
  "created_at": "2025-12-24T..."
}
```

---

## Сценарии использования

### Сценарий 1: Успешный платёж

```
1. Пользователь создаёт аккаунт -> баланс 0
2. Пользователь пополняет баланс -> баланс 1000
3. Пользователь создаёт заказ -> Order Status = "new"
4. Order Service отправляет задачу в orders_queue (Outbox pattern)
5. Payment Service получает задачу -> проверяет баланс (1000 >= 200)
6. Payment Service вычитает деньги -> баланс 800
7. Payment Service отправляет success в payment_events_fanout
8. Order Service получает success -> обновляет Order Status = "finished"
9. API Gateway получает success -> отправляет WebSocket уведомление фронту
10. Фронт показывает зелёное уведомление
```

### Сценарий 2: Неудачный платёж (недостаточно денег)

```
1. Пользователь создаёт заказ на 2000 (баланс только 1000) -> Order Status = "new"
2. Order Service отправляет задачу в orders_queue
3. Payment Service получает задачу -> проверяет баланс (1000 >= 2000)
4. Payment Service отправляет failed в payment_events_fanout
5. Order Service получает failed -> обновляет Order Status = "cancelled"
6. API Gateway получает failed -> отправляет WebSocket уведомление
7. Фронт показывает красное уведомление
```

### Сценарий 3: Resilience (сервис упал и восстановился)

```
1. Заказ создан, отправлен в Outbox таблицу со статусом "pending"
2. Payment Service временно недоступен
3. Order Service продолжает попытки отправить каждые 3 секунды
4. Payment Service восстановился
5. Outbox процессор вычитывает события со статусом "pending"
6. Событие успешно отправляется, статус обновляется на "processed"
7. Гарантия доставки: ни одно сообщение не потеряется!
```

---

## Как работает асинхронная обработка

### Outbox Pattern в Order Service

**Проблема:** Как гарантировать, что если Order создан в БД, то и уведомление полетит в RabbitMQ?

**Решение:** Все в одной транзакции:
1. Вставляем Order в `orders` таблицу
2. Вставляем событие в `outbox` таблицу со статусом `'pending'`
3. Коммитим транзакцию (всё или ничего)
4. Отдельный процессор каждые 3 сек читает `WHERE status = 'pending'`
5. Отправляет в RabbitMQ, потом обновляет статус на `'processed'`

### Inbox Pattern в Payment Service

**Проблема:** Как гарантировать, что сообщение обработано ровно один раз, даже если оно придёт дважды?

**Решение:** Все сообщения вставляются в `inbox_messages` с уникальным `message_id`:
1. Получаем сообщение из `orders_queue`
2. Вставляем в `inbox_messages(message_id, type, payload)` с `ON CONFLICT DO NOTHING`
3. Обрабатываем платёж
4. Отправляем результат в `outbox_events`
5. Коммитим всю транзакцию
6. Только после успешного коммита отправляем `Ack` в RabbitMQ

**Результат:** Если сообщение придёт дважды, второй раз оно проигнорируется (ON CONFLICT), и платёж не будет списан дважды.

---

## Статусы заказов и платежей

### Order Status (order_service/internal/domain/models.go)

```
type OrderStatus string

const (
    OrderStatusNew       OrderStatus = "new"
    OrderStatusFinished  OrderStatus = "finished"
    OrderStatusCancelled OrderStatus = "cancelled"
)
```

**Переходы:**
```
new -> finished    (если платёж успешен)
new -> cancelled   (если платёж не прошёл)
```

### Payment Status (payment_service/internal/domain/models.go)

```
type PaymentStatus string

const (
    PaymentStatusSuccess PaymentStatus = "success"
    PaymentStatusFailed  PaymentStatus = "failed"
)
```

### Outbox Event Status

```
-- В таблице outbox_events
status = 'pending'    -- Ещё не отправлено
status = 'processed'  -- Успешно отправлено в RabbitMQ
```

---

## WebSocket уведомления

### Архитектура

1. **Фронт** подключается к Gateway через WebSocket.
2. **Gateway** хранит в памяти map: `map[userID][]*websocket.Conn`
3. **Payment Service -> Gateway:** Отправляет `payment_events_fanout`
4. **Gateway -> Фронт:** Пересылает по WebSocket клиентам с соответствующим `user_id`

### Код Gateway (api-gateway/cmd/main.go)

```
// WebSocket слушатель (слушает payment_events_fanout от RabbitMQ)
func startRabbitMQListener() {
    msgs, _ := ch.Consume("payments_results_queue", ...)
    for msg := range msgs {
        var event struct {
            OrderID int    `json:"order_id"`
            UserID  int    `json:"user_id"`
            Status  string `json:"status"`  // "success" или "failed"
        }
        json.Unmarshal(msg.Body, &event)
        
        if event.UserID != 0 {
            hub.Broadcast(event.UserID, msg.Body)  // Отправляем в WebSocket
        }
    }
}

// Broadcast отправляет сообщение всем клиентам user'а
func (h *WSHub) Broadcast(userID int, message []byte) {
    h.mu.RLock()
    defer h.mu.RUnlock()
    
    conns := h.clients[userID]
    for _, conn := range conns {
        conn.WriteMessage(websocket.TextMessage, message)
    }
}
```
---

## Проверка работоспособности

**При открытии client/index.html в любом браузере изначально не будет создано ни одного аккаунта.
Требуется создать аккаунт, нажав на кнопку `Create Account`, а дале, после подтверждения создания,
нажать на кнопку `Change User`, чтобы подключиться к нему и выполнять различные действия.**

### Тест 1: Успешный платёж

1. Открой файл client/index.html в любом браузере
2. Создай аккаунт (user_id: 1)
3. Пополни баланс (amount: 1000)
4. Создай заказ (amount: 200)
5. **Ожидаемый результат:**
   - Появится **зелёное** уведомление "Order 1: success"
   - Order Status в БД: `finished`
   - Account Balance: 800

### Тест 2: Неудачный платёж

1. Создай аккаунт (user_id: 2)
2. Пополни баланс (amount: 100)
3. Создай заказ (amount: 500) - больше, чем есть
4. **Ожидаемый результат:**
   - Появится **красное** уведомление "Order 2: failed"
   - Order Status в БД: `cancelled`
   - Account Balance: 100 (не изменился)

### Тест 3: RabbitMQ Management

1. Открой **http://localhost:15672** (user: user, pass: password)
2. Перейди на вкладку **Queues and Streams**
3. **Проверь очереди:**
   - `orders_queue` - сюда Order Service отправляет события
   - `payments_results_queue` - отсюда Gateway получает результаты
4. **Проверь Exchange:**
   - `payment_events_fanout` - fanout exchange (один-ко-всем рассылка)

### Типичные логи (успешный платёж)

```
gozon-order    | Received payment request: OrderID=1 UserID=1 Amount=200
gozon-payment  | Received payment request: OrderID=1 UserID=1 Amount=200
gozon-payment  | Payment SUCCESS for Order 1
gozon-payment  | payments outbox: event 1 (success) sent successfully
gozon-order    | order inbox: received payment result for order 1: success=true
gozon-gateway  | Gateway received event: {"order_id":1,"user_id":1,"status":"success"}
```
---

## Ключевые особенности реализации

### 1. **Гарантия доставки (Inbox/Outbox)**
Ни одно сообщение не потеряется, даже если сервис упадёт

### 2. **Идемпотентность**
Одно и то же сообщение можно обработать много раз - результат одинаков

### 3. **Асинхронность**
Клиент не ждёт ответа платёжной системы - получает уведомление по WebSocket

### 4. **Масштабируемость**
Легко добавить новые сервисы и workers

### 5. **Отказоустойчивость (Resilience)**
Если один сервис упал, другие продолжают работать и восстанавливают состояние

### 6. **Real-time UX**
WebSocket уведомления прилетают мгновенно при статус-изменении

### 7. **Полная типизация (Type Safety)**
Go constants для статусов предотвращают ошибки со строками

