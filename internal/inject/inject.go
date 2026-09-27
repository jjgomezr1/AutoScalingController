// Package inject implementa un modo de inyección de métricas sintéticas para
// EXPERIMENTOS: sobrescribe la CPU (y opcionalmente la latencia) del Snapshot
// con valores de un archivo, sin generar carga real. Si el archivo no existe,
// no hace nada y el controlador usa las métricas reales.
//
// Formato del archivo (una línea): "85" (solo CPU) o "85 2.0" (CPU y latencia).
// Ruta: variable INYECCION_FILE, o /home/ubuntu/inyeccion.txt por defecto.
package inject

import (
	"os"
	"strconv"
	"strings"

	"autoscaling-controller/internal/policy"
)

func rutaArchivo() string {
	if v := os.Getenv("INYECCION_FILE"); v != "" {
		return v
	}
	return "/home/ubuntu/inyeccion.txt"
}

// Aplicar sobrescribe las métricas del Snapshot con los valores del archivo de
// inyección si existe y es válido. Devuelve true si aplicó inyección.
func Aplicar(snap *policy.Snapshot) bool {
	datos, err := os.ReadFile(rutaArchivo())
	if err != nil {
		return false
	}

	campos := strings.Fields(strings.TrimSpace(string(datos)))
	if len(campos) == 0 {
		return false
	}

	cpu, err := strconv.ParseFloat(campos[0], 64)
	if err != nil {
		return false
	}
	snap.CPUMax = cpu

	if len(campos) >= 2 {
		if lat, err := strconv.ParseFloat(campos[1], 64); err == nil {
			snap.LatenciaAvg = lat
		}
	}

	// El valor lo pone quien experimenta, así que es confiable por definición.
	snap.MetricaConfiable = true
	return true
}
