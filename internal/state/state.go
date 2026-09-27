// Package state retiene en memoria los timestamps de la última acción de
// escalado, para calcular los cooldowns. Al reiniciar el proceso se pierde y
// el controlador vuelve a un estado de observación limpio.
package state

import (
	"time"

	"autoscaling-controller/internal/config"
)

// Estado guarda cuándo fue la última subida y la última bajada (cero = nunca).
type Estado struct {
	ultimaSubida time.Time
	ultimaBajada time.Time
}

func Nuevo() *Estado {
	return &Estado{}
}

// RegistrarSubida/RegistrarBajada arrancan el cooldown correspondiente. El
// cooldown se cuenta desde el momento de la acción.
func (e *Estado) RegistrarSubida(ahora time.Time) { e.ultimaSubida = ahora }
func (e *Estado) RegistrarBajada(ahora time.Time) { e.ultimaBajada = ahora }

// EnCooldown indica si aún estamos dentro del enfriamiento de alguna acción.
func (e *Estado) EnCooldown(ahora time.Time) bool {
	if !e.ultimaSubida.IsZero() && ahora.Sub(e.ultimaSubida) < config.CooldownSubida {
		return true
	}
	if !e.ultimaBajada.IsZero() && ahora.Sub(e.ultimaBajada) < config.CooldownBajada {
		return true
	}
	return false
}
