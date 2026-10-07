# === ЭТАП 1: Сборка исполняемого файла ===
FROM golang:1.21-alpine AS builder

# Устанавливаем рабочую папку внутри контейнера
WORKDIR /app

# Копируем файлы зависимостей проекта
COPY go.mod go.sum ./

# Скачиваем библиотеки
RUN go mod download

# Копируем весь исходный код и шаблоны
COPY . .

# Компилируем Go-приложение в один независимый бинарник "main"
RUN CGO_ENABLED=0 GOOS=linux go build -o main .

# === ЭТАП 2: Финальный легковесный образ ===
FROM alpine:latest

WORKDIR /root/

# Копируем скомпилированный бинарник из первого этапа
COPY --from=builder /app/main .

# Копируем папку с HTML шаблонами, так как она нужна для рендеринга фронтенда
COPY --from=builder /app/templates ./templates

# Открываем порт 8080 для внешнего мира
EXPOSE 8080

# Команда для запуска нашего приложения внутри контейнера
CMD ["./main"]
