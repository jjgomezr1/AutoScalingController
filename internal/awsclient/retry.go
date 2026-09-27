package awsclient

import (
	"context"
	"fmt"
	"time"

	"autoscaling-controller/internal/config"
)

// ReintentarConValor ejecuta una operación (T, error) con hasta MaxReintentos
// reintentos separados por EsperaEntreReintentos. Devuelve el primer éxito, o
// el último error si se agotan. Genérica para servir a cualquier tipo de retorno.
func ReintentarConValor[T any](ctx context.Context, nombre string, op func() (T, error)) (T, error) {
	var ultimo error
	var cero T
	totalIntentos := 1 + config.MaxReintentos

	for intento := 1; intento <= totalIntentos; intento++ {
		resultado, err := op()
		if err == nil {
			return resultado, nil
		}
		ultimo = err

		if intento < totalIntentos {
			// Esperar antes del siguiente intento, respetando el context.
			select {
			case <-ctx.Done():
				return cero, fmt.Errorf("%s: context cancelado durante reintentos: %w", nombre, ctx.Err())
			case <-time.After(config.EsperaEntreReintentos):
			}
		}
	}

	return cero, fmt.Errorf("%s: fallaron los %d intentos: %w", nombre, totalIntentos, ultimo)
}

// Reintentar es la variante para operaciones que solo devuelven error.
func Reintentar(ctx context.Context, nombre string, op func() error) error {
	_, err := ReintentarConValor(ctx, nombre, func() (struct{}, error) {
		return struct{}{}, op()
	})
	return err
}
