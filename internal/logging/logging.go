// Package logging registra cada ciclo de forma estructurada, incluso cuando la
// decisión es mantener, para que toda decisión sea explicable.
package logging

import (
	"fmt"
	"time"

	"autoscaling-controller/internal/config"
	"autoscaling-controller/internal/policy"
)

// RegistrarCiclo imprime un renglón con hora, métricas, capacidad, decisión con
// su motivo, y el resultado de la acción. resultado describe qué pasó al actuar
// (ID lanzado/terminado, "sin accion", o un error).
func RegistrarCiclo(ahora time.Time, snap policy.Snapshot, decision policy.Decision, resultado string) {
	fmt.Printf(
		"[%s] cpu_max=%.2f%% latencia=%.3fs hosts_sanos=%d confiable=%t ventana=%s | instancias=%d/%d cooldown=%t | decision=%s (%s) | resultado=%s\n",
		ahora.Format(time.RFC3339),
		snap.CPUMax,
		snap.LatenciaAvg,
		snap.HostsSaludables,
		snap.MetricaConfiable,
		config.VentanaObservacion,
		snap.InstanciasCorriendo,
		config.MaxInstancias,
		snap.EnCooldown,
		decision.Accion,
		decision.Motivo,
		resultado,
	)
}
