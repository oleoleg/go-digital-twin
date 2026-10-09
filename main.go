package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"log"
	"math/rand"
	"net/http"
	"os"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/joho/godotenv"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func homeHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprint(w, "Добро пожаловать в систему мониторинга Цифрового Двойника!")
}

func statusHandler(w http.ResponseWriter, r *http.Request) {
	// ПРОВЕРКА: Если метод НЕ равен GET, возвращаем ошибку 405
	if r.Method != http.MethodGet { // http.MethodGet — это просто строка "GET"
		http.Error(w, "Метод не поддерживается", http.StatusMethodNotAllowed)
		return // Обязательно прерываем выполнение функции!
	}

	// Если это GET-запрос, отдаем наш JSON
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status": "OK", "active_sensors": 1440, "load": "normal"}`))
}

// Описываем структуру спутника.
// Теги json:"..." говорят компилятору, как эти поля будут называться в JSON-формате.
type SatelliteMetrics struct {
	ID        string  `json:"id"`
	Telemetry float64 `json:"telemetry_value"`
	Status    string  `json:"status"`
}

func readSatelliteSensor() (float64, error) {
	if rand.Float64() < 0.3 {
		return 0.0, errors.New("связь со спутником потеряна")
	}
	return 36.6, nil
}

func satelliteHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// 1. Пытаемся прочитать данные с датчика
	temp, err := readSatelliteSensor()
	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(`{"error": "Данные временно недоступны"}`))
		return
	}

	// 2. Если всё ок, упаковываем в структуру
	metrics := SatelliteMetrics{
		ID:        "БЮРО-1440-SPUTNIK",
		Telemetry: temp,
		Status:    "optimal",
	}

	// 3. Переводим в JSON и отправляем
	jsonBytes, _ := json.Marshal(metrics) // Мы временно опустили обработку ошибки маршалинга для простоты
	w.Write(jsonBytes)
}

// //////////////////////////////////////////////////////
// Структура для передачи данных из потока в главный поток
type SensorResult struct {
	SensorName string
	Value      float64
}

// Функция имитирует опрос конкретного датчика.
// Она принимает канал `ch`, куда "выплюнет" результат, как только закончит.
func fetchSensorData(name string, ch chan SensorResult) {
	// Имитируем случайную задержку сети/связи от 100 до 800 миллисекунд
	time.Sleep(time.Duration(100+rand.Intn(1000)) * time.Millisecond)

	// Генерируем случайную телеметрию
	val := rand.Float64() * 100

	// Отправляем результат в канал
	ch <- SensorResult{SensorName: name, Value: val}
}

func aggregateHandler(w http.ResponseWriter, r *http.Request) {
	// Создаем канал, через который горутины передадут нам данные
	resultChan := make(chan SensorResult)

	// Спецификация нашего двойника: нужно опросить 3 подсистемы.
	// Запускаем 3 горутины с помощью ключевого слова `go`
	go fetchSensorData("Навигация (GPS)", resultChan)
	go fetchSensorData("Энергосистема (БАТ)", resultChan)
	go fetchSensorData("Термоконтроль (ТКС)", resultChan)

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintln(w, "=== Сбор телеметрии цифрового двойника ===")

	// Нам нужно дождаться ровно 3 ответов, поэтому пишем цикл на 3 итерации.
	// Чтение из канала `<-resultChan` блокирует код и ждет, пока очередной поток пришлет данные.
	for i := 0; i < 3; i++ {
		select {
		case res := <-resultChan:
			// Если данные пришли из канала — пишем их
			fmt.Fprintf(w, "Получены данные от [%s]: %.2f\n", res.SensorName, res.Value)

		case <-time.After(500 * time.Millisecond):
			// Если данные НЕ пришли за 500 мс — срабатывает этот кейс
			fmt.Fprintln(w, "⚠️ Ошибка: Таймаут ожидания датчика (модуль не ответил вовремя)")
		}
	}

	fmt.Fprintln(w, "Все данные успешно агрегированы!")
}

// ///////////////////////
// Структура для лога в Go и JSON
type SensorLog struct {
	ID         int     `json:"id"`
	SensorName string  `json:"sensor_name"`
	Value      float64 `json:"value"`
	CreatedAt  string  `json:"created_at"`
	IsCritical bool    `json:"is_critical"`
}

var db *sql.DB
var tmpl *template.Template // Добавили глобальный кэш шаблона

func initDB() {
	// 1. Загружаем файлы из .env в окружение приложения
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Ошибка загрузки файла .env")
	}

	// 2. Достаем строку подключения по её ключу из памяти системы
	connStr := os.Getenv("DATABASE_URL")
	if connStr == "" {
		log.Fatal("Переменная DATABASE_URL не задана в конфигурации")
	}

	// 3. Открываем соединение, используя безопасную переменную
	db, err = sql.Open("pgx", connStr)
	if err != nil {
		log.Fatalf("Ошибка конфигурации БД: %v", err)
	}

	err = db.Ping()
	if err != nil {
		log.Fatalf("Не удалось подключиться к PostgreSQL: %v", err)
	}

	fmt.Println("🐘 Успешное и БЕЗОПАСНОЕ подключение к PostgreSQL!")

	query := `
	CREATE TABLE IF NOT EXISTS sensor_logs (
		id SERIAL PRIMARY KEY,
		sensor_name VARCHAR(50) NOT NULL,
		value NUMERIC(10, 2) NOT NULL,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);`
	_, err = db.Exec(query)
	if err != nil {
		log.Fatalf("Ошибка создания таблицы: %v", err)
	}
}

// 1. Хендлер для ЗАПИСИ данных (Имитируем работу датчика)
func saveTelemetryHandler(w http.ResponseWriter, r *http.Request) {
	// Увеличиваем счетчик запросов для пути "/save"
	httpRequestsTotal.WithLabelValues("/save").Inc()

	name := "Датчик-ТКС-1440"
	val := rand.Float64() * 100 // Случайная температура

	// SQL-запрос с защитой от SQL-инъекций (\$1, \$2 — безопасные плейсхолдеры)
	insertQuery := `INSERT INTO sensor_logs (sensor_name, value) VALUES ($1, $2);`

	_, err := db.Exec(insertQuery, name, val)
	if err != nil {
		http.Error(w, "Ошибка записи в БД: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintf(w, "✅ Успешно сохранено! %s показал %.2f °C", name, val)

	// Фиксируем перегрев СТРОГО в момент реального перегрева при генерации!
	if val > 75.0 {
		satelliteOverheatsTotal.Inc()
	}
}

// 2. Хендлер для ЧТЕНИЯ истории из базы данных
func getHistoryHandler(w http.ResponseWriter, r *http.Request) {
	// Делаем выборку последних 10 записей
	rows, err := db.Query("SELECT id, sensor_name, value, created_at FROM sensor_logs ORDER BY id DESC LIMIT 10;")
	if err != nil {
		http.Error(w, "Ошибка чтения из БД: "+err.Error(), http.StatusInternalServerError)
		return
	}
	// Обязательно закрываем rows в конце функции, чтобы не утекли соединения с БД!
	defer rows.Close()

	var history []SensorLog

	// Перебираем строки результата
	for rows.Next() {
		var l SensorLog
		// Scan копирует данные из колонок БД в переменные структуры
		err := rows.Scan(&l.ID, &l.SensorName, &l.Value, &l.CreatedAt)
		if err != nil {
			http.Error(w, "Ошибка сканирования строки: "+err.Error(), http.StatusInternalServerError)
			return
		}
		history = append(history, l)
	}

	// Превращаем полученный слайс структур в JSON и отдаем пользователю
	w.Header().Set("Content-Type", "application/json")
	jsonBytes, _ := json.Marshal(history)
	w.Write(jsonBytes)
}

// ////////////////////////
// 🟢 ВСПЛЫВАЮЩИЙ ГЛАВНЫЙ ХЕНДЛЕР: рендерит HTML-страницу с логами из БД
func dashboardHandler(w http.ResponseWriter, r *http.Request) {

	// Увеличиваем счетчик запросов конкретно для пути "/"
	httpRequestsTotal.WithLabelValues("/").Inc()

	// 1. Достаем последние 15 логов из базы
	rows, err := db.Query("SELECT id, sensor_name, value, to_char(created_at, 'DD.MM.YYYY HH24:MI:SS') FROM sensor_logs ORDER BY id DESC LIMIT 15;")
	if err != nil {
		http.Error(w, "Ошибка чтения из БД: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var history []SensorLog
	for rows.Next() {
		var l SensorLog
		err := rows.Scan(&l.ID, &l.SensorName, &l.Value, &l.CreatedAt)
		if err != nil {
			http.Error(w, "Ошибка сканирования строки: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// 🧠 Проверяем температуру прямо здесь! Если выше 75 градусов — флаг становится true
		l.IsCritical = IsSensorValueCritical(l.Value)

		history = append(history, l)
	}

	// 2. Считаем общее количество записей в таблице для статистики
	var total int
	db.QueryRow("SELECT COUNT(*) FROM sensor_logs;").Scan(&total)

	// 3. Создаем и заполняем нашу Обертку
	data := PageData{
		OperatorName: "Олег",        // Передали одиночную строку
		AppVersion:   "v2.4.1-beta", // Еще одна строка
		TotalLogs:    total,         // Передали одиночное число
		Logs:         history,       // Передали наш массив
	}

	// 3. Соединяем шаблон с массивом данных history и отправляем в ResponseWriter
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	// ⚡ ИСПОЛЬЗУЕМ КЭШ: Просто берем уже готовый скомпилированный шаблон tmpl
	err = tmpl.Execute(w, data)
	if err != nil {
		http.Error(w, "Ошибка рендеринга: "+err.Error(), http.StatusInternalServerError)
	}

}

// ////////////////////////////////
// PageData объединяет ВСЕ данные, которые нужны для отображения на странице
type PageData struct {
	OperatorName string      // Имя инженера
	AppVersion   string      // Версия системы
	TotalLogs    int         // Общее число строк в БД
	Logs         []SensorLog // Наш привычный массив логов
}

// ///////////////////////////////
// Описываем наши SRE-метрики
var (
	// Счетчик общего количества запросов
	httpRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "digital_twin_http_requests_total",
			Help: "Общее количество обработанных HTTP-запросов сервером",
		},
		[]string{"path"}, // Тег (label) для фильтрации по роутам
	)

	// Счетчик перегревов спутника
	satelliteOverheatsTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "digital_twin_satellite_overheats_total",
			Help: "Общее количество зафиксированных критических перегревов (>75°C)",
		},
	)
)

///////////////////////////////////////////

// IsSensorValueCritical проверяет, превышает ли температура норму в 75 градусов.
// Мы вынесли это в отдельную функцию, чтобы её можно было протестировать изолированно от базы данных.
func IsSensorValueCritical(val float64) bool {
	if val > 75.0 {
		return true
	}
	return false
}

func main() {
	initDB()
	defer db.Close()

	var err error
	// 🧠 Компилируем HTML один раз при запуске приложения
	tmpl, err = template.ParseFiles("templates/index.html")
	if err != nil {
		log.Fatalf("Ошибка компиляции шаблона при старте: %v", err)
	}

	http.HandleFunc("/", dashboardHandler)
	http.HandleFunc("/home", homeHandler)
	http.HandleFunc("/status", statusHandler)
	http.HandleFunc("/satellite", satelliteHandler)
	http.HandleFunc("/aggregate", aggregateHandler)
	http.HandleFunc("/save", saveTelemetryHandler)
	http.HandleFunc("/history", getHistoryHandler)
	http.Handle("/metrics", promhttp.Handler())

	fmt.Println("🚀 Сервер с Prometheus-метриками запущен на http://localhost:8080")
	err = http.ListenAndServe(":8080", nil)
	if err != nil {
		fmt.Println("Ошибка запуска сервера:", err)
	}
}
