package main

import (
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
