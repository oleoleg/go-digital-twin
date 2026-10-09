package main

import (
	"html/template"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// Тестируем нормальную температуру спутника
func TestIsSensorValueCritical_Normal(t *testing.T) {
	inputValue := 45.5
	expected := false

	result := IsSensorValueCritical(inputValue)

	if result != expected {
		// Если результат не совпал с ожидаемым, вызываем ошибку теста
		t.Errorf("Ошибка! Для температуры %.1f ожидали %t, но получили %t", inputValue, expected, result)
	}
}

// Тестируем перегрев спутника
func TestIsSensorValueCritical_Critical(t *testing.T) {
	inputValue := 89.2
	expected := true

	result := IsSensorValueCritical(inputValue)

	if result != expected {
		t.Errorf("Ошибка! Для температуры %.1f ожидали %t, но получили %t", inputValue, expected, result)
	}
}

////////////////////////////////////////

func TestIsSensorValueCritical_TableDriven(t *testing.T) {
	// 1. Определяем "таблицу" тестов.
	// Это срез анонимных структур, где мы описываем параметры сценария.
	tests := []struct {
		name       string  // Имя сценария (чтобы сразу понять какой тест упал)
		inputValue float64 // Что подаем на вход функции
		expected   bool    // Что функция ДОЛЖНА вернуть
	}{
		{name: "Обычная холодная температура", inputValue: 23.4, expected: false},
		{name: "Нормальная рабочая температура", inputValue: 55.0, expected: false},
		{name: "Граничное значение (ровно 75)", inputValue: 75.0, expected: false}, // 75.0 еще норма, строго больше 75 — критично
		{name: "Минимальный перегрев", inputValue: 75.1, expected: true},
		{name: "Критический перегрев модуля", inputValue: 120.5, expected: true},
	}

	// 2. В цикле перебираем всю таблицу строк за строкой
	for _, tc := range tests {
		// t.Run запускает каждый элемент таблицы как изолированный "подтест" (Subtest)
		t.Run(tc.name, func(t *testing.T) {
			result := IsSensorValueCritical(tc.inputValue)

			if result != tc.expected {
				t.Errorf("Провал [%s]: для %.1f ожидали %t, но получили %t",
					tc.name, tc.inputValue, tc.expected, result)
			}
		})
	}
}

// BenchmarkIsSensorValueCritical замеряет скорость работы нашей функции
func BenchmarkIsSensorValueCritical(b *testing.B) {
	// b.N генерируется фреймворком Go автоматически.
	// Движок будет запускать цикл до тех пор, пока не получит статистически точное время.
	for i := 0; i < b.N; i++ {
		// Вызываем тестируемую функцию с любым значением
		IsSensorValueCritical(65.4)
	}
}

// ////////////////////////////
// sync.Once гарантирует, что база данных подключится ровно ОДИН раз
// для всех бенчмарков, чтобы не перегружать СУБД при тестах.
var initDbOnce sync.Once

func BenchmarkDashboardHandler(b *testing.B) {
	// 1. Инициализируем базу данных, загружаем .env и парсем шаблон (только при первом запуске бенчмарка)
	initDbOnce.Do(func() {
		initDB()
		tmpl, _ = template.ParseFiles("templates/index.html")
	})

	// 2. Создаем фейковый запрос методом GET на коренной URL "/"
	req, err := http.NewRequest("GET", "/", nil)
	if err != nil {
		b.Fatal(err)
	}

	// 3. Сбрасываем таймер бенчмарка непосредственно перед запуском цикла.
	// Время, потраченное на подключение к базе данных выше, не должно портить нам статистику!
	b.ResetTimer()

	// 4. Запускаем главный цикл замера производительности
	for i := 0; i < b.N; i++ {
		// httptest.NewRecorder() — это фейковый "получатель ответа" (вместо реального браузера)
		rr := httptest.NewRecorder()

		// Напрямую вызываем наш хендлер, передавая ему фейковый ответ и фейковый запрос
		dashboardHandler(rr, req)

		// Дополнительная SRE-проверка внутри бенчмарка:
		// Если сервер вдруг ответил не 200 OK, значит что-то сломалось, прерываем тест.
		if rr.Code != http.StatusOK {
			b.Fatalf("Сервер вернул статус %d вместо 200", rr.Code)
		}
	}
}
