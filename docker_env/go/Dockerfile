
# Финальный образ (Runner)
FROM debian:bookworm-slim

WORKDIR /app

# ca-certificates и tzdata — для внешних HTTPS запросов и таймзон.
# fonts-dejavu-core — кириллический TTF для выгрузки в PDF: в TCPDF монолита
# dejavusans был вшит в библиотеку, здесь его надо принести с собой.
RUN apt-get update \
    && apt-get install -y ca-certificates tzdata fonts-dejavu-core \
    && rm -rf /var/lib/apt/lists/*

# Открываем порт, на котором работает микросервис
EXPOSE 8087

# Запуск приложения (бинарник main, собранный на хосте с помощью go build cmd/main.go)
CMD ["./main"]
