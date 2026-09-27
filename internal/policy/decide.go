// Package policy contiene la lógica de decisión. Es PURO: no toca AWS, no lee
// el reloj ni imprime, por lo que se prueba de forma determinista con go test.
package policy

import (
	"fmt"

	"autoscaling-controller/internal/config"
)

// Accion es el resultado de una decisión: mantener, subir o bajar.
type Accion int

const (
	Mantener Accion = iota
	Subir
	Bajar
)

func (a Accion) String() string {
	switch a {
	case Subir:
		return "subir"
	case Bajar:
		return "bajar"
	default:
		return "mantener"
	}
}

// Snapshot es el estado observado en un ciclo que Decide() interpreta.
// CPUMax es el máximo entre instancias del promedio de CPU (0-100), LatenciaAvg
// es el TargetResponseTime del ALB en segundos y HostsSaludables el
// HealthyHostCount del Target Group.
type Snapshot struct {
	InstanciasCorriendo int
	CPUMax              float64
	MetricaConfiable    bool
	EnCooldown          bool
	LatenciaAvg         float64
	HostsSaludables     int
}

type Decision struct {
	Accion Accion
	Motivo string
}

// Decide resuelve la acción a tomar. Subir es fácil (basta CPU o latencia
// altas), bajar es conservador (todo debe estar tranquilo y los hosts sanos).
// Las guardas van primero: mínimo, confiabilidad, cooldown.
func Decide(s Snapshot) Decision {
	// Estar por debajo del mínimo pesa más que cualquier métrica o cooldown.
	if s.InstanciasCorriendo < config.MinInstancias {
		return Decision{
			Accion: Subir,
			Motivo: "por debajo del minimo de instancias; se sube para garantizar la capacidad base (ignora metricas y cooldown)",
		}
	}

	if !s.MetricaConfiable {
		return Decision{
			Accion: Mantener,
			Motivo: "metrica no confiable (obsoleta o faltante); se mantiene el estado actual",
		}
	}

	if s.EnCooldown {
		return Decision{
			Accion: Mantener,
			Motivo: "en cooldown tras una accion previa; se mantiene el estado actual",
		}
	}

	if s.CPUMax > config.CPUSubir || s.LatenciaAvg > config.LatenciaSubir {
		if s.InstanciasCorriendo >= config.MaxInstancias {
			return Decision{
				Accion: Mantener,
				Motivo: "carga alta pero ya se alcanzo el maximo de instancias; se mantiene",
			}
		}
		return Decision{
			Accion: Subir,
			Motivo: fmt.Sprintf("subir: CPU=%.1f%% (umbral %.0f) o latencia=%.2fs (umbral %.2f)",
				s.CPUMax, config.CPUSubir, s.LatenciaAvg, config.LatenciaSubir),
		}
	}

	if s.CPUMax < config.CPUBajar && s.LatenciaAvg < config.LatenciaBajar {
		if s.InstanciasCorriendo <= config.MinInstancias {
			return Decision{
				Accion: Mantener,
				Motivo: "carga baja pero ya se esta en el minimo de instancias; se mantiene",
			}
		}
		if s.HostsSaludables != s.InstanciasCorriendo {
			return Decision{
				Accion: Mantener,
				Motivo: fmt.Sprintf("carga baja pero no todos los hosts estan sanos (%d sanos de %d); no se baja",
					s.HostsSaludables, s.InstanciasCorriendo),
			}
		}
		return Decision{
			Accion: Bajar,
			Motivo: fmt.Sprintf("bajar: CPU=%.1f%% y latencia=%.2fs bajas, %d hosts sanos == %d instancias",
				s.CPUMax, s.LatenciaAvg, s.HostsSaludables, s.InstanciasCorriendo),
		}
	}

	return Decision{
		Accion: Mantener,
		Motivo: "metricas dentro de la banda normal; capacidad adecuada, se mantiene",
	}
}
